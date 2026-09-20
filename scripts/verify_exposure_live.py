"""使用隔离 Docker 服务和真实 API 进程验证曝光链路，退出时清理测试资源。"""

import argparse
import base64
import concurrent.futures
from datetime import datetime
import json
import hashlib
import math
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid


# run 执行命令并返回输出；命令失败时终止验证，所有外部命令均设置超时。
def run(args, *, cwd=None, env=None, timeout=180):
    result = subprocess.run(args, cwd=cwd, env=env, text=True, encoding="utf-8",
                            errors="replace", capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"command failed: {args[0]}\n{result.stdout}\n{result.stderr}")
    return result.stdout.strip()


# request 通过真实 HTTP 连接发送请求，返回状态码与 JSON，不输出访问令牌。
def request(url, method="GET", body=None, headers=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method=method,
                                 headers={"Content-Type": "application/json", **(headers or {})})
    try:
        response = urllib.request.urlopen(req, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        content = response.read()
        return response.status, json.loads(content) if content else None


# wait_until 在限定时间内等待可验证条件成立；连接尚未就绪时继续检查。
def wait_until(check, timeout=45):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if check():
                return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.2)
    raise RuntimeError("等待真实服务状态超时")


# require 在断言失败时终止验证，避免优化模式跳过测试条件。
def require(condition, message):
    if not condition:
        raise AssertionError(message)


# main 创建隔离服务，运行曝光及可选视频向量真实链路，保存脱敏结果并清理全部测试资源。
def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", default="docs/exposure-live-results.json")
    parser.add_argument("--embedding", action="store_true", help="同时验证视频向量存储链路")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    project = "videofeed-exposure-e2e-" + uuid.uuid4().hex[:8]
    compose = ["docker", "compose", "-p", project, "-f", str(root / "scripts/compose.exposure-test.yaml")]
    evidence = {"project": project, "started_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                "checks": [], "status": "running"}
    process = None
    worker_process = None
    worker_log = None
    log_file = None
    failure = None
    with tempfile.TemporaryDirectory(prefix="videofeed-exposure-") as work:
        work = Path(work)
        try:
            print("Starting isolated PostgreSQL / RabbitMQ / Redis", flush=True)
            run(compose + ["up", "-d", "--wait", "--wait-timeout", "120"])

            # port 读取当前测试项目的动态宿主端口。
            def port(service, container_port):
                return int(run(compose + ["port", service, str(container_port)]).rsplit(":", 1)[1])

            # sql 在独立测试数据库执行 SQL 并返回文本结果。
            def sql(statement):
                return run(compose + ["exec", "-T", "postgres", "psql", "-U", "exposure_test",
                                      "-d", "exposure_test", "-v", "ON_ERROR_STOP=1", "-At", "-c", statement])

            # passed 记录已执行且通过的检查，不把跳过计入成功。
            def passed(name):
                evidence["checks"].append(name)
                print("PASS: " + name, flush=True)

            pg_port, amqp_port, management_port, redis_port = port("postgres", 5432), port("rabbitmq", 5672), port("rabbitmq", 15672), port("redis", 6379)

            # listeners_ready 确认 AMQP 与管理端口真正接受连接，避免仅节点存活时过早运行测试。
            def listeners_ready():
                for ready_port in (amqp_port, management_port):
                    with socket.create_connection(("127.0.0.1", ready_port), timeout=2):
                        pass
                return True

            wait_until(listeners_ready)
            env = os.environ.copy()
            env.update({"VIDEOFEED_TEST_POSTGRES_URL": f"postgres://exposure_test:exposure_test_only@127.0.0.1:{pg_port}/exposure_test?sslmode=disable",
                        "VIDEOFEED_TEST_AMQP_URL": f"amqp://exposure_test:exposure_test_only@127.0.0.1:{amqp_port}/"})
            evidence["versions"] = {"go": run(["go", "version"]), "docker": run(["docker", "version", "--format", "{{.Server.Version}}"]),
                                    "postgres": sql("SHOW server_version"), "rabbitmq": run(compose + ["exec", "-T", "rabbitmq", "rabbitmqctl", "version"]),
                                    "redis": run(compose + ["exec", "-T", "redis", "redis-server", "--version"])}
            packages = ["./internal/infrastructure/persistence/exposure", "./internal/infrastructure/mq"]
            names = ["TestExposurePostgresLive", "TestExposureQueueLive"]
            if args.embedding:
                packages.append("./internal/infrastructure/persistence/embedding")
                names.extend(["TestEmbeddingPostgresLive", "TestEmbeddingQueueLive"])
            result = run(["go", "test", "-count=1", "-v", *packages, "-run", "^(" + "|".join(names) + ")$"], cwd=root, env=env)
            for name in names:
                require(f"--- PASS: {name}" in result and f"--- SKIP: {name}" not in result, name + " 未实际通过")
                passed(name)
            require(sql("SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'exposure_test_%'") == "0", "隔离 schema 未清理")
            require(sql("SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'embedding_test_%'") == "0", "向量测试 schema 未清理")
            management = f"http://127.0.0.1:{management_port}/api"
            auth = {"Authorization": "Basic " + base64.b64encode(b"exposure_test:exposure_test_only").decode()}

            # admin 调用隔离 RabbitMQ 管理接口并校验响应状态。
            def admin(path, method="GET", body=None):
                status, value = request(management + path, method, body, auth)
                require(200 <= status < 300, f"RabbitMQ management: {path} status={status}")
                return value

            wait_until(lambda: len(admin("/connections")) == 0)
            require(not admin("/queues"), "Go 测试遗留了消息队列")
            require(not [item for item in admin("/exchanges") if item["name"].startswith("videofeed.test.")], "Go 测试遗留交换机")
            passed("isolated_schema_queue_exchange_connection_cleanup")
            binary = work / ("api.exe" if os.name == "nt" else "api")
            run(["go", "build", "-o", str(binary), "./cmd/server"], cwd=root)
            (work / "configs").mkdir()
            shutil.copyfile(root / "configs/config.yaml", work / "configs/config.yaml")
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                api_port = listener.getsockname()[1]
            env.update({"DATABASE_URL": env["VIDEOFEED_TEST_POSTGRES_URL"], "JWT_SECRET": "exposure-live-test-only-jwt-secret-2026",
                        "SERVER_HOST": "127.0.0.1", "SERVER_PORT": str(api_port), "REDIS_ADDR": f"127.0.0.1:{redis_port}",
                        "REDIS_PASSWORD": "", "REDIS_DB": "0", "RABBITMQ_URL": env["VIDEOFEED_TEST_AMQP_URL"],
                        "RABBITMQ_INTERACTION_EXCHANGE": "exposure.e2e.interaction", "RABBITMQ_ACTION_CHANGED_QUEUE": "exposure.e2e.actions",
                        "RABBITMQ_ACTION_CHANGED_ROUTING": "actions", "RABBITMQ_VIDEO_EXCHANGE": "exposure.e2e.video",
                        "RABBITMQ_VIDEO_PUBLISHED_QUEUE": "exposure.e2e.videos", "RABBITMQ_VIDEO_PUBLISHED_ROUTING": "videos",
                        "RABBITMQ_VIDEO_EMBEDDING_QUEUE": "exposure.e2e.embedding",
                        "RABBITMQ_EXPOSURE_EXCHANGE": "exposure.e2e.exposure", "RABBITMQ_VIEW_EVENT_RECORDED_QUEUE": "exposure.e2e.views",
                        "RABBITMQ_VIEW_EVENT_RECORDED_ROUTING": "views"})
            base = f"http://127.0.0.1:{api_port}"

            # stop_api 停止本脚本创建的 API 进程并关闭日志句柄。
            def stop_api():
                nonlocal process, log_file
                if process is not None:
                    process.terminate()
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)
                    process = None
                if log_file is not None:
                    log_file.close()
                    log_file = None

            # start_api 启动实际应用入口，并等待真实健康检查端点响应。
            def start_api():
                nonlocal process, log_file
                log_file = (work / "api.log").open("a", encoding="utf-8")
                process = subprocess.Popen([str(binary)], cwd=work, env=env, stdout=log_file, stderr=subprocess.STDOUT)

                # healthy 检查进程存活及 HTTP 健康状态，启动失败时报告本地测试日志。
                def healthy():
                    if process.poll() is not None:
                        raise RuntimeError("API startup failed: " + (work / "api.log").read_text(encoding="utf-8"))
                    return request(base + "/health")[0] == 200

                wait_until(healthy)

            # stop_worker 停止本脚本的独立消费者进程并释放日志句柄。
            def stop_worker():
                nonlocal worker_process, worker_log
                if worker_process is not None:
                    worker_process.terminate()
                    try:
                        worker_process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        worker_process.kill()
                        worker_process.wait(timeout=5)
                    worker_process = None
                if worker_log is not None:
                    worker_log.close()
                    worker_log = None

            # start_worker 启动当前代码构建的独立消费者，等待三类队列订阅完成。
            def start_worker():
                nonlocal worker_process, worker_log
                worker_log = (work / "worker.log").open("a", encoding="utf-8")
                worker_process = subprocess.Popen([str(worker_binary)], cwd=work, env=env, stdout=worker_log, stderr=subprocess.STDOUT)
                wait_until(lambda: all(admin("/queues/%2F/" + name).get("consumers", 0) == 1
                                       for name in ("exposure.e2e.actions", "exposure.e2e.videos", "exposure.e2e.embedding")))

            start_api()
            if args.embedding:
                worker_binary = work / ("worker.exe" if os.name == "nt" else "worker")
                run(["go", "build", "-o", str(worker_binary), "./cmd/worker"], cwd=root)
                start_worker()
            status, user = request(base + "/api/users", "POST", {"account": "exposure_e2e", "password": "Test-only-password-2026", "nickname": "曝光联调"})
            require(status == 201, "注册失败")
            status, session = request(base + "/api/sessions", "POST", {"account": "exposure_e2e", "password": "Test-only-password-2026"})
            require(status == 200, "登录失败")
            jwt = {"Authorization": "Bearer " + session["access_token"]}
            status, video = request(base + "/api/videos", "POST", {"title": "曝光联调视频", "description": "isolated test", "media_url": "https://example.com/test.mp4", "cover_url": "https://example.com/test.jpg"}, jwt)
            require(status == 201, "视频发布失败")
            video_id = video["id"]
            passed("actual_api_startup_registration_login_video_publish")

            if args.embedding:
                # embedding_row 从实际数据库读取指定视频的本地模型记录。
                def embedding_row():
                    result = sql(f"SELECT row_to_json(v) FROM video_embedding v WHERE video_id={video_id} AND model='hash-ngram-v1'")
                    return json.loads(result) if result else None

                # publish_video 向真实视频交换机发送指定发布消息，验证独立队列绑定。
                def publish_video(title, description, raw=None):
                    body = raw if raw is not None else json.dumps({"event_id": uuid.uuid4().hex, "video_id": video_id, "title": title, "description": description})
                    result = admin("/exchanges/%2F/exposure.e2e.video/publish", "POST", {"properties": {"delivery_mode": 2}, "routing_key": "videos", "payload": body, "payload_encoding": "string"})
                    require(result["routed"], "发布事件未路由")

                wait_until(lambda: embedding_row() is not None)
                first_embedding = embedding_row()
                vector = first_embedding["embedding_json"]
                require(first_embedding["dimension"] == len(vector) == 128 and abs(math.sqrt(sum(v*v for v in vector))-1) < 1e-10, "实际发布未生成归一化 128 维向量")
                require(first_embedding["text_hash"] == hashlib.sha256("曝光联调视频\nisolated test".encode()).hexdigest(), "发布视频文本哈希错误")
                wait_until(lambda: admin("/queues/%2F/exposure.e2e.embedding").get("consumers", 0) == 1)
                queue = admin("/queues/%2F/exposure.e2e.embedding")
                require(queue["durable"] and queue["consumers"] == 1, "向量队列未持久化或消费者未启动")
                # 相同内容重投必须执行覆盖写，但保持创建时间和唯一记录。
                publish_video("曝光联调视频", "isolated test")
                wait_until(lambda: embedding_row()["updated_at"] != first_embedding["updated_at"])
                require(embedding_row()["created_at"] == first_embedding["created_at"] and sql(f"SELECT count(*) FROM video_embedding WHERE video_id={video_id}") == "1", "重复消息破坏唯一性或创建时间")
                publish_video("", "", raw="invalid-json")
                publish_video("改变后的标题", "新的简介")
                changed_hash = hashlib.sha256("改变后的标题\n新的简介".encode()).hexdigest()
                wait_until(lambda: embedding_row()["text_hash"] == changed_hash)
                require(embedding_row()["embedding_json"] != vector, "内容更新未改变向量")
                passed("video_publish_embedding_128d_hash_replay_update_and_poison_rejection")
                # 暂时令真实表拒绝更新，再解除约束，验证消费者重投成功。
                sql("ALTER TABLE video_embedding ADD CONSTRAINT embedding_retry_block CHECK (dimension < 0) NOT VALID")
                retry_hash = hashlib.sha256("数据库恢复重试\n内容".encode()).hexdigest()
                publish_video("数据库恢复重试", "内容")
                wait_until(lambda: "failed to save video embedding" in (work / "worker.log").read_text(encoding="utf-8") or "embedding_retry_block" in (work / "worker.log").read_text(encoding="utf-8"))
                require(embedding_row()["text_hash"] == changed_hash, "失败更新覆盖了旧记录")
                sql("ALTER TABLE video_embedding DROP CONSTRAINT embedding_retry_block")
                wait_until(lambda: embedding_row()["text_hash"] == retry_hash)
                require(sql(f"SELECT count(*) FROM video_embedding WHERE video_id={video_id}") == "1", "重投生成重复行")
                passed("real_embedding_storage_failure_requeue_and_recovery")
                # 将独立 worker 的 Redis 指向不可达地址，向量队列仍应独立工作。
                stop_api()
                stop_worker()
                env["REDIS_ADDR"] = "127.0.0.1:1"
                start_api()
                start_worker()
                publish_video("Redis 不可达", "向量仍保存")
                independent_hash = hashlib.sha256("Redis 不可达\n向量仍保存".encode()).hexdigest()
                wait_until(lambda: embedding_row()["text_hash"] == independent_hash)
                passed("embedding_worker_operates_with_redis_unavailable")
                stop_api()
                stop_worker()
                env["REDIS_ADDR"] = f"127.0.0.1:{redis_port}"
                start_api()

            # report 向真实应用提交观看事件，并核对预期 HTTP 状态。
            def report(kind="exposed", status=201, headers=None, **fields):
                actual, value = request(base + "/api/video-view-events", "POST", {"video_id": video_id, "scene": "timeline", "request_id": "same-request", "event_type": kind, **fields}, jwt if headers is None else headers)
                require(actual == status, f"view event expected={status} got={actual} response={value}")
                return value

            first = report()
            with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
                concurrent_responses = list(pool.map(lambda _: report(), range(32)))
            require({item["exposure"]["exposure_count"] for item in concurrent_responses} == set(range(2, 34)), "HTTP 并发曝光计数缺失")
            require(all(item["exposure"]["first_exposed_at"] == first["exposure"]["first_exposed_at"] for item in concurrent_responses), "首次曝光时间变化")
            require(sql("SELECT exposure_count FROM exposures") == "33", "数据库聚合计数不符")
            completed = report("complete", completed=False, watch_ms=1234, user_id=999)
            require(completed["event"]["completed"] and completed["event"]["user_id"] == user["id"] and "exposure" not in completed, "完播或身份映射错误")
            played = report("play", watch_ms=88)
            skipped = report("skip")
            expected = {item["event"]["id"]: item for item in [first, *concurrent_responses, completed, played, skipped]}
            passed("real_http_concurrency_33_exposures_four_event_types_jwt_identity")

            # messages 从真实队列取出并确认消息，用于核对发布内容及数量。
            def messages():
                return admin("/queues/%2F/exposure.e2e.views/get", "POST", {"count": 100, "ackmode": "ack_requeue_false", "encoding": "auto", "truncate": 100000})

            queue = admin("/queues/%2F/exposure.e2e.views")
            exchange = admin("/exchanges/%2F/exposure.e2e.exposure")
            bindings = admin("/bindings/%2F/e/exposure.e2e.exposure/q/exposure.e2e.views")
            require(queue["durable"] and exchange["durable"] and exchange["type"] == "topic" and any(item["routing_key"] == "views" for item in bindings), "真实消息拓扑不符")
            delivered = messages()
            require(len(delivered) == 36, f"消息数量错误：{len(delivered)}")
            payloads = [json.loads(item["payload"]) for item in delivered]
            require(set(expected) == {item["view_event_id"] for item in payloads}, "消息未关联全部落库事件")
            require(len({item["event_id"] for item in payloads}) == 36, "消息编号重复")
            require(all(item["properties"]["delivery_mode"] == 2 and item["properties"]["message_id"] == value["event_id"] and value["user_id"] == user["id"] and value["video_id"] == video_id for item, value in zip(delivered, payloads)), "消息属性或用户视频字段错误")
            require(all(item["recorded_at"] and item["occurred_at"] for item in payloads), "消息时间缺失")
            stored = {item["id"]: item for item in json.loads(sql("SELECT json_agg(v) FROM video_view_events v"))}
            for payload in payloads:
                response = expected[payload["view_event_id"]]
                row = stored[payload["view_event_id"]]
                for field in ("user_id", "video_id", "scene", "request_id", "event_type", "watch_ms", "completed"):
                    require(payload[field] == response["event"][field] == row[field], f"HTTP、数据库、消息字段不一致：{field}")
                require(datetime.fromisoformat(payload["recorded_at"]) == datetime.fromisoformat(row["created_at"]), "消息落库时间不一致")
                require(payload.get("exposure_count", 0) == response.get("exposure", {}).get("exposure_count", 0), "消息曝光计数与响应不一致")
            passed("real_topic_durable_queue_binding_and_36_persistent_messages")
            report(status=401, headers={})
            report(status=401, headers={"Authorization": "Bearer invalid"})
            report(status=400, scene=" ")
            report("progress", status=400)
            report(status=400, watch_ms=-1)
            report(status=404, video_id=999999)
            sql(f"UPDATE videos SET status=3 WHERE id={video_id}")
            report(status=404)
            sql(f"UPDATE videos SET status=2 WHERE id={video_id}")
            require(sql("SELECT count(*) FROM video_view_events") == "36" and not messages(), "无效请求产生副作用")
            sql("ALTER TABLE exposures ADD CONSTRAINT e2e_failure CHECK (last_scene <> 'fail')")
            error = report(status=500, scene="fail")
            require(error == {"error": "internal server error"}, "存储错误泄漏")
            require(sql("SELECT count(*) FROM video_view_events") == "36" and sql("SELECT exposure_count FROM exposures") == "33" and not messages(), "HTTP 事务失败未回滚或仍发布消息")
            sql("ALTER TABLE exposures DROP CONSTRAINT e2e_failure")
            passed("http_400_401_404_500_no_side_effects_transaction_rollback")
            durable = report("play", request_id="durable-before-restart")
            run(compose + ["stop", "rabbitmq"])
            disconnected = report("play", request_id="broker-disconnected")
            stop_api()
            start_api()
            disabled = report("play", request_id="broker-disabled-at-startup")
            require("rabbitmq disabled" in (work / "api.log").read_text(encoding="utf-8"), "未验证消息服务启动失败回退")
            run(compose + ["up", "-d", "--wait", "--wait-timeout", "120", "rabbitmq"])
            # 动态宿主端口在容器重新启动后可能变化，重新发现后再访问管理接口和重启 API。
            amqp_port, management_port = port("rabbitmq", 5672), port("rabbitmq", 15672)
            management = f"http://127.0.0.1:{management_port}/api"
            env["RABBITMQ_URL"] = f"amqp://exposure_test:exposure_test_only@127.0.0.1:{amqp_port}/"
            wait_until(listeners_ready)
            wait_until(lambda: request(management + "/health/checks/virtual-hosts", headers=auth)[0] == 200)
            persisted = messages()
            require(len(persisted) == 1 and json.loads(persisted[0]["payload"])["view_event_id"] == durable["event"]["id"], "持久化消息重启后丢失或发布失败产生了消息")
            for value in (disconnected, disabled):
                require(sql(f"SELECT count(*) FROM video_view_events WHERE id={value['event']['id']}") == "1", "消息故障导致流水丢失")
            passed("broker_disconnect_startup_failure_fallback_and_restart_durability")
            stop_api()
            start_api()
            recovered = report("play", request_id="after-recovery")
            delivered = messages()
            require(len(delivered) == 1 and json.loads(delivered[0]["payload"])["view_event_id"] == recovered["event"]["id"], "重启 API 后消息发布未恢复")
            require(sql("SELECT count(*) FROM video_view_events") == "40", "最终流水数量错误")
            passed("api_restart_restores_publish_and_final_40_rows")
            evidence["counts"] = {"http_view_events_saved": 40, "exposure_count": 33, "initial_messages": 36, "expected_unpublished_during_failure": 2}
            stop_api()
            wait_until(lambda: len(admin("/connections")) == 0)
            passed("api_process_exit_releases_amqp_connections")
            evidence["status"] = "passed"
        except Exception as error:
            evidence["status"] = "failed"
            evidence["error"] = str(error)
            failure = error
        finally:
            if worker_process is not None:
                worker_process.kill()
                worker_process.wait(timeout=15)
            if worker_log is not None:
                worker_log.close()
            if process is not None:
                process.kill()
                process.wait(timeout=15)
            if log_file is not None:
                log_file.close()
            try:
                run(compose + ["down", "--volumes", "--remove-orphans"])
                require(not run(["docker", "ps", "-aq", "--filter", f"label=com.docker.compose.project={project}"]), "残留测试容器")
                require(not run(["docker", "volume", "ls", "-q", "--filter", f"label=com.docker.compose.project={project}"]), "残留测试数据卷")
                evidence["cleanup"] = "passed"
            except Exception as error:
                evidence["cleanup"] = "failed"
                evidence["status"] = "failed"
                failure = failure or error
            evidence["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%S%z")
            output = root / args.output
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if failure:
        raise failure
    print("All real-service checks passed; isolated resources removed.", flush=True)


if __name__ == "__main__":
    main()
