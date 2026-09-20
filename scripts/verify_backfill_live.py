"""使用隔离 PostgreSQL、Redis、RabbitMQ 和真实 HTTP 验证关注回填，并清理测试资源。"""
import concurrent.futures
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import uuid
from verify_exposure_live import run, request, wait_until, require


# main 启动隔离服务和当前代码构建的后端，验证回填及故障重试，保存脱敏证据并清理资源。
def main():
    root = Path(__file__).resolve().parent.parent
    project = 'videofeed-backfill-' + uuid.uuid4().hex[:8]
    compose = ['docker', 'compose', '-p', project, '-f', str(root / 'scripts/compose.exposure-test.yaml')]
    evidence = {'project': project, 'started_at': time.strftime('%Y-%m-%dT%H:%M:%S%z'), 'checks': [], 'status': 'running'}
    process = None
    log = None
    with tempfile.TemporaryDirectory(prefix='videofeed-backfill-') as temp:
        work = Path(temp)
        try:
            print('Starting isolated real services', flush=True)
            run(compose + ['up', '-d', '--wait', '--wait-timeout', '120'])

            # port 读取测试容器随机映射的本机端口。
            def port(service, number):
                return int(run(compose + ['port', service, str(number)]).rsplit(':', 1)[1])

            # sql 在本次独立数据库中执行固定测试语句。
            def sql(statement):
                return run(compose + ['exec', '-T', 'postgres', 'psql', '-U', 'exposure_test', '-d', 'exposure_test', '-v', 'ON_ERROR_STOP=1', '-At', '-c', statement])

            # redis 在本次独立 Redis 中读取或注入测试状态。
            def redis(*args):
                return run(compose + ['exec', '-T', 'redis', 'redis-cli', '--raw', *map(str, args)])

            # passed 保存通过的断言，不输出账号密码或令牌。
            def passed(name):
                evidence['checks'].append(name)
                print('PASS: ' + name, flush=True)

            env = os.environ.copy()
            env.update({'DATABASE_URL': f'postgres://exposure_test:exposure_test_only@127.0.0.1:{port("postgres",5432)}/exposure_test?sslmode=disable',
                        'JWT_SECRET': 'backfill-isolated-test-only-secret-2026', 'REDIS_ADDR': f'127.0.0.1:{port("redis",6379)}', 'REDIS_PASSWORD': '', 'REDIS_DB': '0',
                        'RABBITMQ_URL': f'amqp://exposure_test:exposure_test_only@127.0.0.1:{port("rabbitmq",5672)}/', 'SERVER_HOST': '127.0.0.1'})
            with socket.socket() as listener:
                listener.bind(('127.0.0.1', 0))
                env['SERVER_PORT'] = str(listener.getsockname()[1])
            evidence['versions'] = {'postgres': sql('SHOW server_version'), 'redis': run(compose + ['exec','-T','redis','redis-server','--version']), 'go': run(['go','version'])}
            binary = work / ('api.exe' if os.name == 'nt' else 'api')
            run(['go', 'build', '-o', str(binary), './cmd/server'], cwd=root)
            (work / 'configs').mkdir()
            shutil.copyfile(root / 'configs/config.yaml', work / 'configs/config.yaml')
            log = (work / 'api.log').open('w', encoding='utf-8')
            process = subprocess.Popen([str(binary)], cwd=work, env=env, stdout=log, stderr=subprocess.STDOUT)
            base = 'http://127.0.0.1:' + env['SERVER_PORT']
            wait_until(lambda: request(base + '/health')[0] == 200)

            # account 使用真实注册与登录接口创建账号并返回身份与令牌头。
            def account(name):
                body = {'account': name, 'password': 'Test-only-password-2026', 'nickname': name}
                status, user = request(base + '/api/users', 'POST', body)
                require(status == 201, 'registration failed')
                status, token = request(base + '/api/sessions', 'POST', body)
                require(status == 200, 'login failed')
                return user['id'], {'Authorization': 'Bearer ' + token['access_token']}

            viewer, auth = account('backfill_viewer')
            author, author_auth = account('backfill_author')
            big, big_auth = account('backfill_big')
            stranger, stranger_auth = account('backfill_stranger')
            passed('actual_api_health_registration_login')
            for who in [author_auth, big_auth]:
                status, video = request(base + '/api/videos', 'POST', {'title':'backfill video','description':'isolated','media_url':'https://example.com/video.mp4','cover_url':'https://example.com/cover.jpg'}, who)
                require(status == 201, 'publish failed')
            # 批量作品用于 100 条上限及排除不可见作品，不代表通过上传创建的媒体。
            sql(f"INSERT INTO videos(author_id,title,description,media_url,cover_url,status,published_at,created_at,updated_at) SELECT {author}, 'seed-'||n,'','https://example.com/v.mp4','https://example.com/c.jpg',2,now()-n*interval '1 minute',now(),now() FROM generate_series(1,104) n; INSERT INTO videos(author_id,title,description,media_url,cover_url,status,published_at,created_at,updated_at) VALUES ({author},'hidden','','x','x',1,now(),now(),now()),({author},'unpublished','','x','x',2,NULL,now(),now());")
            expected = [int(x) for x in sql(f'SELECT id FROM videos WHERE author_id={author} AND status=2 AND published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT 100').splitlines()]
            key = f'feed:following:inbox:v1:{viewer}'
            require(redis('ZCARD', key) == '0', 'inbox should be empty before follow')
            headers = {**auth, 'Idempotency-Key': 'follow-backfill-live'}
            path = base + f'/api/users/me/following/{author}'
            status, result = request(path, 'PUT', headers=headers)
            require(status == 200 and result['following'], 'follow failed')
            members = redis('ZREVRANGE', key, 0, -1).splitlines()
            require([int(x.split(':')[0]) for x in members] == expected, 'recent 100 or ordering mismatch')
            require(int(redis('TTL', key)) >= 29*24*3600, 'TTL missing')
            passed('follow_backfills_exact_latest_100_published_videos_with_ttl')
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                results = list(pool.map(lambda _: request(path, 'PUT', headers=headers), range(16)))
            require(all(x[0] == 200 for x in results), 'concurrent retry failed')
            require(redis('ZCARD', key) == '100', 'duplicate inbox records')
            require(sql(f'SELECT following_count FROM user_relation_stat WHERE user_id={viewer}') == '1', 'following count increased')
            require(sql(f'SELECT count(*) FROM user_follow WHERE user_id={viewer} AND target_user_id={author}') == '1', 'duplicate relationship')
            passed('16_concurrent_idempotent_follow_retries_preserve_counts_and_inbox')
            status, page = request(base + '/api/feed-items?scene=following&limit=10', headers=auth)
            require(status == 200 and [x['video_id'] for x in page['items']] == expected[:10], 'feed missing small creator')
            status, page2 = request(base + '/api/feed/query', 'POST', {'scene':'following','limit':10,'cursor':page['next_cursor']}, auth)
            require(status == 200 and [x['video_id'] for x in page2['items']] == expected[10:20], 'pagination mismatch')
            status, anon = request(base + '/api/feed-items?scene=following')
            require(status == 401, 'anonymous access accepted')
            status, other = request(base + '/api/feed-items?scene=following', headers=stranger_auth)
            require(status == 200 and not other['items'], 'cross-user leak')
            passed('following_get_post_pagination_authentication_and_user_isolation')
            status, _ = request(path, 'DELETE', headers=auth)
            require(status == 200, 'unfollow failed')
            status, page = request(base + '/api/feed-items?scene=following', headers=auth)
            require(status == 200 and not page['items'] and redis('ZCARD',key)=='100', 'unfollow visibility failed')
            passed('unfollow_filters_stale_inbox_without_removing_cached_members')
            sql(f'INSERT INTO user_relation_stat(user_id,following_count,follower_count,created_at,updated_at) VALUES ({big},0,9999,now(),now()) ON CONFLICT (user_id) DO UPDATE SET follower_count=9999')
            require(sql(f'SELECT follower_count FROM user_relation_stat WHERE user_id={big}') == '9999', 'big creator fixture not initialized')
            status, _ = request(base + f'/api/relations/{big}', 'PUT', headers={**auth,'Idempotency-Key':'big-live'})
            require(status == 200 and redis('ZCARD',key)=='100', 'big creator incorrectly backfilled')
            require(sql(f'SELECT follower_count FROM user_relation_stat WHERE user_id={big}')=='10000','threshold setup failed')
            passed('10000_follower_threshold_skips_backfill_via_relation_alias')
            redis('DEL',key)
            redis('SET',key,'wrong-type-fault')
            retry_headers={**auth,'Idempotency-Key':'failure-retry-live'}
            status, failure = request(path,'PUT',headers=retry_headers)
            require(status==500 and failure=={'error':'internal server error'},'cache fault did not return 500')
            require(sql(f'SELECT status FROM user_follow WHERE user_id={viewer} AND target_user_id={author}')=='1','relationship rolled back')
            saved_count=sql(f'SELECT following_count FROM user_relation_stat WHERE user_id={viewer}')
            status, fallback=request(base+'/api/feed-items?scene=following',headers=auth)
            require(status==200 and fallback['items'],'cache error did not fall back')
            redis('DEL',key)
            status,_=request(path,'PUT',headers=retry_headers)
            require(status==200 and redis('ZCARD',key)=='100','retry failed to recover inbox')
            require(sql(f'SELECT following_count FROM user_relation_stat WHERE user_id={viewer}')==saved_count,'retry changed count')
            passed('redis_wrongtype_500_keeps_committed_follow_database_fallback_and_same_key_retry')
            # 预置 1000 条索引以验证回填事务的容量裁剪。
            redis('EVAL',"for i=1,1000 do redis.call('ZADD',KEYS[1],i,'old-'..i) end return 1",1,key)
            status,_=request(path,'PUT',headers=retry_headers)
            require(status==200 and redis('ZCARD',key)=='1000','inbox cap failed')
            passed('real_redis_transaction_trims_inbox_to_1000')
            evidence['status']='passed'
        except Exception as error:
            evidence['status']='failed'
            evidence['error']=str(error)
            try:
                evidence['service_logs']=run(compose+['logs','--tail','40','rabbitmq'])
            except Exception:
                pass
            raise
        finally:
            if process is not None:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
            if log is not None:
                log.close()
            try:
                run(compose+['down','-v','--remove-orphans'])
                require(not run(['docker','ps','-a','--filter',f'label=com.docker.compose.project={project}','-q']), 'test container remains')
                require(not run(['docker','volume','ls','--filter',f'label=com.docker.compose.project={project}','-q']), 'test volume remains')
                evidence['cleanup']='passed'
            except Exception:
                evidence['cleanup']='failed'
                evidence['status']='failed'
                raise
            finally:
                evidence['finished_at']=time.strftime('%Y-%m-%dT%H:%M:%S%z')
                (root/'docs/backfill-live-results.json').write_text(json.dumps(evidence,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print('All backfill live checks passed; test resources removed.',flush=True)


if __name__ == '__main__':
    main()
