"""使用隔离数据库、缓存和消息队列验证独立 API 与 worker，结束后清理进程及测试卷。"""

import base64
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import uuid

from verify_exposure_live import run, request, require, wait_until


# main 构建并启动真实进程，验证无消费者积压、三类消费和重启恢复，保存脱敏证据并清理资源。
def main():
    root = Path(__file__).resolve().parent.parent
    project = "videofeed-worker-" + uuid.uuid4().hex[:8]
    compose = ["docker", "compose", "-f", str(root / "scripts/compose.exposure-test.yaml"), "-p", project]
    evidence = {"project": project, "checks": [], "status": "running", "started_at": time.strftime("%Y-%m-%dT%H:%M:%S%z")}
    processes = {}
    logs = []
    failure = None
    with tempfile.TemporaryDirectory(prefix="videofeed-worker-") as folder:
        work = Path(folder)
        try:
            run(compose + ["up", "-d", "--wait", "--wait-timeout", "120"])

            # port 读取隔离服务的随机本机端口。
            def port(service, number):
                return run(compose + ["port", service, str(number)]).rsplit(":", 1)[1]

            # sql 在专用数据库执行断言或故障注入，不访问现有业务数据库。
            def sql(statement):
                return run(compose + ["exec", "-T", "postgres", "psql", "-U", "exposure_test", "-d", "exposure_test", "-v", "ON_ERROR_STOP=1", "-At", "-c", statement])

            # passed 记录通过的联调断言。
            def passed(name):
                evidence["checks"].append(name)
                print("PASS " + name, flush=True)

            env = os.environ.copy()
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                api_port = listener.getsockname()[1]
            env.update({"DATABASE_URL": f"postgres://exposure_test:exposure_test_only@127.0.0.1:{port('postgres',5432)}/exposure_test?sslmode=disable",
                        "JWT_SECRET": "worker-live-test-only-jwt-secret-2026", "SERVER_HOST": "127.0.0.1", "SERVER_PORT": str(api_port),
                        "REDIS_ADDR": "127.0.0.1:" + port("redis",6379), "REDIS_PASSWORD": "", "REDIS_DB": "0",
                        "RABBITMQ_URL": f"amqp://exposure_test:exposure_test_only@127.0.0.1:{port('rabbitmq',5672)}/"})
            management = "http://127.0.0.1:" + port("rabbitmq",15672) + "/api"
            auth = {"Authorization": "Basic " + base64.b64encode(b"exposure_test:exposure_test_only").decode()}

            # admin 调用本次隔离 RabbitMQ 的管理接口。
            def admin(path, method="GET", body=None):
                status, value = request(management + path, method, body, auth)
                require(200 <= status < 300, f"management {path}: {status}")
                return value

            (work / "configs").mkdir()
            shutil.copyfile(root / "configs/config.docker.yaml", work / "configs/config.yaml")
            binaries = {}
            for name, package in (("api", "server"), ("worker", "worker")):
                binaries[name] = work / (name + (".exe" if os.name == "nt" else ""))
                run(["go", "build", "-o", str(binaries[name]), "./cmd/" + package], cwd=root)

            # start 启动指定真实入口并保留日志供失败诊断。
            def start(name):
                log = (work / (name + ".log")).open("a", encoding="utf-8")
                logs.append(log)
                processes[name] = subprocess.Popen([str(binaries[name])], cwd=work, env=env, stdout=log, stderr=subprocess.STDOUT)

            # stop 只停止本脚本创建的进程，超时后强制结束以保证清理。
            def stop(name):
                process = processes.pop(name, None)
                if process is not None:
                    process.terminate()
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)

            queues = ["videofeed.interaction.action_changed", "videofeed.video.published", "videofeed.video.embedding"]

            # consumers 返回三个实际业务队列的消费者数量。
            def consumers():
                return [admin("/queues/%2F/" + queue).get("consumers", 0) for queue in queues]

            base = f"http://127.0.0.1:{api_port}"
            start("api")
            wait_until(lambda: request(base + "/health")[0] == 200)
            require(consumers() == [0, 0, 0], "API 启动了消费者")
            passed("api_only_has_zero_consumers")

            # account 通过真实注册和登录接口创建测试身份。
            def account(name):
                status, user = request(base + "/api/users", "POST", {"account": name, "password": "Test-only-password-2026", "nickname": name})
                require(status == 201, "注册失败")
                status, session = request(base + "/api/sessions", "POST", {"account": name, "password": "Test-only-password-2026"})
                require(status == 200, "登录失败")
                return user["id"], {"Authorization": "Bearer " + session["access_token"]}

            author, author_auth = account("worker_author")
            viewer, viewer_auth = account("worker_viewer")
            status, _ = request(base + f"/api/users/me/following/{author}", "PUT", headers=viewer_auth)
            require(status == 200, "关注失败")
            status, video = request(base + "/api/videos", "POST", {"title": "独立任务联调", "description": "worker test", "media_url": "https://example.com/test.mp4", "cover_url": "https://example.com/test.jpg"}, author_auth)
            require(status == 201, "发布失败")
            video_id = video["id"]
            status, _ = request(base + f"/api/videos/{video_id}/like", "PUT", headers={**viewer_auth, "Idempotency-Key": "worker-like"})
            require(status == 200, "点赞失败")
            require(sql("SELECT count(*) FROM video_embedding") == "0", "API 生成了向量")
            require(sql("SELECT count(*) FROM interaction_action") == "0", "API 直接处理了异步互动")
            wait_until(lambda: all(admin("/queues/%2F/" + queue).get("messages_ready", 0) >= 1 for queue in queues))
            passed("api_publishes_three_queue_backlogs_without_consuming")
            # 同时重启 API 与 worker，验证共享迁移可并发重复执行。
            stop("api")
            start("api")
            start("worker")
            wait_until(lambda: request(base + "/health")[0] == 200)
            wait_until(lambda: consumers() == [1, 1, 1])
            wait_until(lambda: sql(f"SELECT count(*) FROM video_embedding WHERE video_id={video_id}") == "1")
            wait_until(lambda: sql(f"SELECT like_count FROM video_stat WHERE video_id={video_id}") == "1")
            wait_until(lambda: run(compose + ["exec", "-T", "redis", "redis-cli", "ZCARD", f"feed:following:inbox:v1:{viewer}"]) == "1")
            passed("concurrent_migrations_and_worker_action_fanout_embedding")
            require(sql("SELECT count(*) FROM pg_indexes WHERE indexname='idx_video_timeline'") == "1", "时间线索引缺失")
            require(sql("SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('interaction_action_idempotency','user_follow_idempotency')") == "2", "幂等表缺失")
            passed("target_timeline_index_and_idempotency_tables_preserved")
            stop("worker")
            wait_until(lambda: consumers() == [0, 0, 0])
            require(request(base + "/health")[0] == 200, "停止 worker 影响 API")
            status, _ = request(base + f"/api/videos/{video_id}/favorite", "PUT", headers={**viewer_auth, "Idempotency-Key": "worker-favorite"})
            require(status == 200, "收藏发布失败")
            require(sql(f"SELECT favorite_count FROM video_stat WHERE video_id={video_id}") == "0", "停止后事件仍被消费")
            start("worker")
            wait_until(lambda: sql(f"SELECT favorite_count FROM video_stat WHERE video_id={video_id}") == "1")
            wait_until(lambda: consumers() == [1, 1, 1])
            passed("worker_restart_drains_backlog_while_api_remains_available")
            stop("worker")
            stop("api")
            wait_until(lambda: len(admin("/connections")) == 0)
            passed("process_exit_releases_amqp_connections")
            evidence["status"] = "passed"
        except Exception as error:
            evidence["status"] = "failed"
            evidence["error"] = str(error)
            failure = error
            for name in ("api", "worker"):
                path = work / (name + ".log")
                if path.exists():
                    print(path.read_text(encoding="utf-8", errors="replace")[-4000:], flush=True)
        finally:
            for process in processes.values():
                process.kill()
                process.wait(timeout=15)
            for log in logs:
                log.close()
            run(compose + ["down", "-v", "--remove-orphans"])
            require(not run(["docker", "ps", "-aq", "--filter", "label=com.docker.compose.project=" + project]), "残留测试容器")
            require(not run(["docker", "volume", "ls", "-q", "--filter", "label=com.docker.compose.project=" + project]), "残留测试卷")
            evidence["cleanup"] = "passed"
            evidence["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%S%z")
            (root / "docs/worker-live-results.json").write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if failure:
        raise failure


if __name__ == "__main__":
    main()
