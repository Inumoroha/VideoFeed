# VideoFeed 本地联调脚本，覆盖鉴权、上传、发布、互动、消息、指标和 Worker 异步持久化。
$ErrorActionPreference = 'Continue'
$Base = 'http://127.0.0.1:9000'
$InternalToken = 'integration-internal-token'
$results = @()

# Check 记录一项断言结果并输出状态。
function Check($name, $ok, $detail) {
    $script:results += [pscustomobject]@{ check = $name; passed = [bool]$ok; detail = $detail }
    $tag = if ($ok) { 'PASS' } else { 'FAIL' }
    Write-Output ("{0} {1} :: {2}" -f $tag, $name, $detail)
}

# Invoke-Api 发送 JSON 请求，返回状态码和解析后的响应体。
function Invoke-Api($Method, $Path, $Body, $Token, $ExtraHeaders) {
    $headers = @{}
    if ($Token) { $headers['Authorization'] = "Bearer $Token" }
    if ($ExtraHeaders) { foreach ($k in $ExtraHeaders.Keys) { $headers[$k] = $ExtraHeaders[$k] } }
    $call = @{ Method = $Method; Uri = "$Base$Path"; Headers = $headers; UseBasicParsing = $true; TimeoutSec = 30 }
    if ($null -ne $Body) {
        $call['Body'] = ($Body | ConvertTo-Json -Depth 6 -Compress)
        $call['ContentType'] = 'application/json'
    }
    try {
        $resp = Invoke-WebRequest @call
        $parsed = $null
        try { $parsed = $resp.Content | ConvertFrom-Json } catch { $parsed = $null }
        return @{ status = [int]$resp.StatusCode; raw = $resp.Content; json = $parsed }
    } catch {
        $r = $_.Exception.Response
        if ($r) {
            $reader = New-Object System.IO.StreamReader($r.GetResponseStream())
            $text = $reader.ReadToEnd()
            $parsed = $null
            try { $parsed = $text | ConvertFrom-Json } catch { $parsed = $null }
            return @{ status = [int]$r.StatusCode; raw = $text; json = $parsed }
        }
        return @{ status = 0; raw = $_.Exception.Message; json = $null }
    }
}

# New-Png 写入可通过 MIME 探测的最小 PNG 文件。
function New-Png($path) {
    $b64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
    [System.IO.File]::WriteAllBytes($path, [Convert]::FromBase64String($b64))
}

# Invoke-Upload 发送 multipart 文件请求，返回状态码和解析后的响应体。
function Invoke-Upload($Path, $FieldName, $FilePath, $Kind, $Token) {
    $boundary = [guid]::NewGuid().ToString('N')
    $fileBytes = [System.IO.File]::ReadAllBytes($FilePath)
    $ms = New-Object System.IO.MemoryStream
    $w = New-Object System.IO.BinaryWriter($ms)
    $enc = [System.Text.Encoding]::UTF8
    if ($Kind) {
        $w.Write($enc.GetBytes("--$boundary`r`nContent-Disposition: form-data; name=`"kind`"`r`n`r`n$Kind`r`n"))
    }
    $name = [System.IO.Path]::GetFileName($FilePath)
    $w.Write($enc.GetBytes("--$boundary`r`nContent-Disposition: form-data; name=`"$FieldName`"; filename=`"$name`"`r`nContent-Type: application/octet-stream`r`n`r`n"))
    $w.Write($fileBytes)
    $w.Write($enc.GetBytes("`r`n--$boundary--`r`n"))
    $w.Flush()
    $body = $ms.ToArray()
    $w.Dispose(); $ms.Dispose()
    $headers = @{}
    if ($Token) { $headers['Authorization'] = "Bearer $Token" }
    try {
        $resp = Invoke-WebRequest -Method Post -Uri "$Base$Path" -Body $body -ContentType "multipart/form-data; boundary=$boundary" -Headers $headers -UseBasicParsing -TimeoutSec 60
        $parsed = $null
        try { $parsed = $resp.Content | ConvertFrom-Json } catch { $parsed = $null }
        return @{ status = [int]$resp.StatusCode; json = $parsed; raw = $resp.Content }
    } catch {
        $r = $_.Exception.Response
        if ($r) {
            $reader = New-Object System.IO.StreamReader($r.GetResponseStream())
            $text = $reader.ReadToEnd()
            $parsed = $null
            try { $parsed = $text | ConvertFrom-Json } catch { $parsed = $null }
            return @{ status = [int]$r.StatusCode; json = $parsed; raw = $text }
        }
        return @{ status = 0; json = $null; raw = $_.Exception.Message }
    }
}

# Wait-ViewerState 等待异步消费后的 Feed 查看者状态达到预期，超时返回最后一次读取结果。
function Wait-ViewerState($VideoID, $Token, $Liked, $Favorited) {
    $deadline = (Get-Date).AddSeconds(10)
    $item = $null
    do {
        $feed = Invoke-Api Get '/api/feed-items?scene=timeline&limit=50' $null $Token
        $item = $feed.json.items | Where-Object { $_.video_id -eq $VideoID } | Select-Object -First 1
        if ($item -and $item.liked -eq $Liked -and $item.favorited -eq $Favorited) { return $item }
        Start-Sleep -Milliseconds 200
    } while ((Get-Date) -lt $deadline)
    return $item
}

$suffix = [guid]::NewGuid().ToString('N').Substring(0, 8)
$uname = "it$suffix"

# 1. 鉴权
$reg = Invoke-Api Post '/api/users' @{ account = $uname; password = 'Passw0rd!'; nickname = "it-nick-$suffix" } $null
Check 'register' ($reg.status -eq 201 -or $reg.status -eq 200) "status=$($reg.status)"

$login = Invoke-Api Post '/api/sessions' @{ account = $uname; password = 'Passw0rd!' } $null
$token = $login.json.access_token
Check 'login returns access_token' ($login.status -eq 200 -and $token) "status=$($login.status) token=$([bool]$token)"

$me = Invoke-Api Get '/api/users/me' $null $token
Check 'users/me' ($me.status -eq 200 -and $me.json.account -eq $uname) "status=$($me.status) account=$($me.json.account)"

$noAuth = Invoke-Api Get '/api/users/me' $null $null
Check 'users/me rejects anonymous' ($noAuth.status -eq 401) "status=$($noAuth.status)"

# 2. 上传校验
$tmp = Join-Path $env:TEMP "vf-it-$suffix"
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$png = Join-Path $tmp 'cover.png'
New-Png $png

$badExt = Join-Path $tmp 'evil.exe'
[System.IO.File]::WriteAllBytes($badExt, [System.IO.File]::ReadAllBytes($png))
$r = Invoke-Upload '/api/uploads' 'file' $badExt 'cover' $token
Check 'upload rejects bad extension' ($r.status -eq 400) "status=$($r.status) body=$($r.raw)"

$cover = Invoke-Upload '/api/uploads' 'file' $png 'cover' $token
Check 'upload cover png' ($cover.status -eq 201) "status=$($cover.status) url=$($cover.json.url)"

$anonUpload = Invoke-Upload '/api/uploads' 'file' $png 'cover' $null
Check 'upload requires auth' ($anonUpload.status -eq 401) "status=$($anonUpload.status)"

# 3. 发布
$video = Invoke-Api Post '/api/videos' @{
    title       = "it video $suffix"
    description = 'integration test'
    media_url   = $cover.json.url
    cover_url   = $cover.json.url
} $token @{ 'Idempotency-Key' = "it-video-$suffix" }
$videoID = $video.json.id
Check 'create video' ($video.status -eq 201 -and $videoID) "status=$($video.status) id=$videoID"

$replay = Invoke-Api Post '/api/videos' @{
    title       = "it video $suffix"
    description = 'integration test'
    media_url   = $cover.json.url
    cover_url   = $cover.json.url
} $token @{ 'Idempotency-Key' = "it-video-$suffix" }
Check 'create video idempotent replay' ($replay.json.id -eq $videoID) "replay=$($replay.json.id) first=$videoID"

# 4. Feed 与查看者互动状态
$feed = Invoke-Api Get '/api/feed-items?scene=timeline&limit=50' $null $token
$item = $feed.json.items | Where-Object { $_.video_id -eq $videoID } | Select-Object -First 1
Check 'timeline feed contains new video' ($feed.status -eq 200 -and $item) "status=$($feed.status) items=$($feed.json.items.Count)"
Check 'feed item exposes liked/favorited' ($null -ne $item.liked -and $null -ne $item.favorited) "liked=$($item.liked) favorited=$($item.favorited)"

$anonFeed = Invoke-Api Get '/api/feed-items?scene=timeline&limit=5' $null $null
Check 'anonymous timeline allowed' ($anonFeed.status -eq 200) "status=$($anonFeed.status)"

$anonReco = Invoke-Api Get '/api/feed-items?scene=recommend&limit=5' $null $null
Check 'recommend requires login' ($anonReco.status -eq 401) "status=$($anonReco.status)"

$reco = Invoke-Api Get '/api/feed-items?scene=recommend&limit=5' $null $token
Check 'recommend works for logged in viewer' ($reco.status -eq 200) "status=$($reco.status) items=$($reco.json.items.Count)"

# 5. 互动
$like = Invoke-Api Put "/api/videos/$videoID/like" @{} $token
Check 'like video' ($like.status -eq 200 -and $like.json.active -eq $true) "status=$($like.status) active=$($like.json.active) like_count=$($like.json.like_count)"

$fav = Invoke-Api Put "/api/videos/$videoID/favorite" @{} $token
Check 'favorite video' ($fav.status -eq 200 -and $fav.json.active -eq $true) "status=$($fav.status) active=$($fav.json.active)"

$item2 = Wait-ViewerState $videoID $token $true $true
Check 'feed reflects like+favorite' ($item2.liked -eq $true -and $item2.favorited -eq $true) "liked=$($item2.liked) favorited=$($item2.favorited)"

$unlike = Invoke-Api Delete "/api/videos/$videoID/like" $null $token
$item3 = Wait-ViewerState $videoID $token $false $true
Check 'unlike clears feed state' ($unlike.status -eq 200 -and $item3.liked -eq $false) "unlike=$($unlike.status) liked=$($item3.liked)"

$comment = Invoke-Api Post "/api/videos/$videoID/comments" @{ content = "it comment $suffix" } $token @{ 'Idempotency-Key' = "it-comment-$suffix" }
Check 'create comment' ($comment.status -eq 201 -or $comment.status -eq 200) "status=$($comment.status) comment_count=$($comment.json.comment_count)"

$comments = Invoke-Api Get "/api/videos/$videoID/comments?limit=10" $null $null
Check 'list comments' ($comments.status -eq 200 -and $comments.json.items.Count -ge 1) "status=$($comments.status) count=$($comments.json.items.Count)"

# 6. 内部消息与令牌
$msg = Invoke-Api Post '/internal/messages' @{
    user_id = 1; type = 'like'; title = 'liked'; content = 'liked your video'; event_id = "it-evt-$suffix"
    actor_id = 77; actor_nickname = 'it-actor'; actor_avatar_url = 'https://cdn.test/a.png'
} $null @{ 'X-Internal-Token' = $InternalToken; 'Idempotency-Key' = "it-msg-$suffix" }
Check 'internal message carries actor fields' ($msg.status -eq 201 -and $msg.json.actor_id -eq 77 -and $msg.json.actor_nickname -eq 'it-actor') "status=$($msg.status) actor_id=$($msg.json.actor_id) nickname=$($msg.json.actor_nickname) avatar=$($msg.json.actor_avatar_url)"

$msgReplay = Invoke-Api Post '/internal/messages' @{
    user_id = 1; type = 'like'; title = 'changed'; content = 'changed'; event_id = "it-evt-$suffix"
    actor_id = 77; actor_nickname = 'it-actor'; actor_avatar_url = 'https://cdn.test/a.png'
} $null @{ 'X-Internal-Token' = $InternalToken; 'Idempotency-Key' = "it-msg-$suffix" }
Check 'message idempotent replay keeps actor' ($msgReplay.json.id -eq $msg.json.id -and $msgReplay.json.actor_id -eq 77) "replay_id=$($msgReplay.json.id) actor_id=$($msgReplay.json.actor_id)"

$msgNoToken = Invoke-Api Post '/internal/messages' @{ user_id = 1; type = 'like'; title = 'x'; content = 'y'; event_id = "it-evt2-$suffix" } $null
Check 'internal message rejects missing token' ($msgNoToken.status -eq 401) "status=$($msgNoToken.status)"

$msgBadToken = Invoke-Api Post '/internal/messages' @{ user_id = 1; type = 'like'; title = 'x'; content = 'y'; event_id = "it-evt3-$suffix" } $null @{ 'X-Internal-Token' = 'wrong' }
Check 'internal message rejects wrong token' ($msgBadToken.status -eq 401) "status=$($msgBadToken.status)"

$recoInternal = Invoke-Api Post '/internal/recommendation-candidates' @{ user_id = 1; scene = 'recommend'; request_id = "it-reco-$suffix"; limit = 5 } $null
Check 'internal recommendation reachable' ($recoInternal.status -eq 200) "status=$($recoInternal.status)"

# 7. 指标
$metrics = Invoke-WebRequest -Uri "$Base/metrics" -UseBasicParsing -TimeoutSec 10
$m = $metrics.Content
Check 'api /metrics exposed' ($metrics.StatusCode -eq 200) "http=$($metrics.StatusCode) bytes=$($m.Length)"
Check 'metrics use videofeed namespace' ($m -match 'videofeed_http_requests_total') 'videofeed_http_requests_total present'
Check 'metrics have no gcfeed leakage' (-not ($m -match 'gcfeed_')) 'no gcfeed_ prefix'
$routeLine = ($m -split "`n") | Where-Object { $_ -match 'videofeed_http_requests_total\{.*route="/api/feed-items"' } | Select-Object -First 1
Check 'metrics record /api/feed-items route' ([bool]$routeLine) ("$routeLine" -replace "`r", '')

$wm = Invoke-WebRequest -Uri 'http://127.0.0.1:9091/metrics' -UseBasicParsing -TimeoutSec 10
Check 'worker /metrics exposed on 9091' ($wm.StatusCode -eq 200) "http=$($wm.StatusCode) bytes=$($wm.Content.Length)"

# 8. Worker 异步持久化
Start-Sleep -Seconds 5
$feed4 = Invoke-Api Get '/api/feed-items?scene=timeline&limit=10' $null $token
$item4 = $feed4.json.items | Where-Object { $_.video_id -eq $videoID } | Select-Object -First 1
Check 'interaction counts readable after worker consumed events' ($null -ne $item4.like_count -and $null -ne $item4.favorite_count) "like_count=$($item4.like_count) favorite_count=$($item4.favorite_count) comment_count=$($item4.comment_count)"

$health = Invoke-WebRequest -Uri "$Base/health" -UseBasicParsing -TimeoutSec 5
Check 'health endpoint' ($health.StatusCode -eq 200) "http=$($health.StatusCode) body=$($health.Content)"

# 汇总断言结果并写入临时证据文件。
$passed = @($results | Where-Object { $_.passed }).Count
$failed = @($results | Where-Object { -not $_.passed }).Count
Write-Output ""
Write-Output "================ SUMMARY ================"
Write-Output "passed=$passed failed=$failed total=$($results.Count)"
$evidence = [pscustomobject]@{
    ran_at   = (Get-Date).ToString('s')
    api      = $Base
    worker   = 'http://127.0.0.1:9091'
    passed   = $passed
    failed   = $failed
    total    = $results.Count
    video_id = $videoID
    account  = $uname
    checks   = $results
}
$out = Join-Path $env:TEMP "vf-integration-$suffix.json"
$evidence | ConvertTo-Json -Depth 6 | Set-Content -Encoding UTF8 $out
Write-Output "evidence: $out"
if ($failed -gt 0) {
    Write-Output "FAILED CHECKS:"
    $results | Where-Object { -not $_.passed } | ForEach-Object { Write-Output ("  - {0} :: {1}" -f $_.check, $_.detail) }
}
Remove-Item -LiteralPath $png, $badExt -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
if ($failed -gt 0) { exit 1 }
