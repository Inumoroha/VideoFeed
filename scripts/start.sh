#!/usr/bin/env bash
set -euo pipefail

# 定位脚本目录和项目根目录，避免从其他工作目录启动时路径失效。
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# 本项目的 Go 服务位于根目录，Web 前端位于 web 子目录。
API_DIR="${ROOT_DIR}"
WEB_DIR="${ROOT_DIR}/web"
CONFIG_FILE="${API_DIR}/configs/config.yaml"

# 从默认端口开始寻找可用端口，也允许调用者通过环境变量指定起始端口。
API_PORT_START="${API_PORT:-9000}"
WEB_PORT_START="${WEB_PORT:-5173}"

# 实际尝试绑定本机端口，能够识别 Windows 保留端口和已有监听进程。
port_available() {
    local port="$1"
    if command -v node >/dev/null 2>&1; then
        node -e 'const net = require("net"); const port = Number(process.argv[1]); const bind = () => { const server = net.createServer(); server.once("error", () => process.exit(1)); server.listen(port, "0.0.0.0", () => server.close(() => process.exit(0))); }; const probe = net.createConnection({ host: "127.0.0.1", port }); probe.once("connect", () => { probe.destroy(); process.exit(1); }); probe.once("error", () => { probe.destroy(); bind(); }); probe.setTimeout(200, () => { probe.destroy(); bind(); });' "${port}" >/dev/null 2>&1
        return
    fi
    if command -v nc >/dev/null 2>&1; then
        ! nc -z 127.0.0.1 "${port}" >/dev/null 2>&1
        return
    fi
    ! (echo >/dev/tcp/127.0.0.1/"${port}") >/dev/null 2>&1
}

# 从指定端口开始选择第一个可用端口，连续端口均不可用时返回失败。
find_available_port() {
    local port="$1"
    local end=$((port + 20))
    while ((port < end)); do
        if port_available "${port}"; then
            echo "${port}"
            return 0
        fi
        port=$((port + 1))
    done
    return 1
}

# 校验端口参数为 1 到 65535 之间的整数。
valid_port() {
    local port="$1"
    [[ "${port}" =~ ^[0-9]+$ ]] && ((port >= 1 && port <= 65535))
}

# 终止服务进程及其子进程，避免 Windows 下 npm 或 go run 留下孤儿进程。
stop_process_tree() {
    local pid="$1"
    if command -v taskkill.exe >/dev/null 2>&1; then
        taskkill.exe //PID "${pid}" //T //F >/dev/null 2>&1 || true
        return
    fi
    kill "${pid}" 2>/dev/null || true
}

# 启动前检查运行时命令和必要文件。
if ! command -v go >/dev/null 2>&1; then
    echo "go command is required"
    exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
    echo "npm command is required"
    exit 1
fi
if ! command -v node >/dev/null 2>&1; then
    echo "node command is required"
    exit 1
fi
if [[ ! -f "${CONFIG_FILE}" ]]; then
    echo "config file not found: ${CONFIG_FILE}"
    exit 1
fi
if [[ ! -f "${WEB_DIR}/package.json" ]]; then
    echo "web package file not found: ${WEB_DIR}/package.json"
    exit 1
fi
if ! valid_port "${API_PORT_START}"; then
    echo "invalid API_PORT: ${API_PORT_START}"
    exit 1
fi
if ! valid_port "${WEB_PORT_START}"; then
    echo "invalid WEB_PORT: ${WEB_PORT_START}"
    exit 1
fi

if ! API_PORT="$(find_available_port "${API_PORT_START}")"; then
    echo "no available API port found near ${API_PORT_START}"
    exit 1
fi
if ! WEB_PORT="$(find_available_port "${WEB_PORT_START}")"; then
    echo "no available web port found near ${WEB_PORT_START}"
    exit 1
fi
export SERVER_PORT="${API_PORT}"
export WEB_PORT
export VITE_API_TARGET="http://127.0.0.1:${API_PORT}"

# 依赖目录不存在或 Vite 可执行文件缺失时自动安装前端依赖。
if [[ ! -f "${WEB_DIR}/node_modules/.bin/vite" && ! -f "${WEB_DIR}/node_modules/.bin/vite.cmd" ]]; then
    echo "Installing web dependencies"
    if ! (cd "${WEB_DIR}" && npm install); then
        echo "failed to install web dependencies"
        exit 1
    fi
fi

# 停止已启动的服务，等待子进程退出并保留原始退出码。
cleanup() {
    local code=$?
    trap - EXIT INT TERM

    if [[ -n "${API_PID:-}" ]] && kill -0 "${API_PID}" 2>/dev/null; then
        stop_process_tree "${API_PID}"
    fi
    if [[ -n "${WEB_PID:-}" ]] && kill -0 "${WEB_PID}" 2>/dev/null; then
        stop_process_tree "${WEB_PID}"
    fi

    wait "${API_PID:-}" "${WEB_PID:-}" 2>/dev/null || true
    rm -f "${API_LOG:-}" 2>/dev/null || true
    exit "${code}"
}

trap cleanup EXIT INT TERM

# 启动 API，并在端口竞争导致绑定失败时自动切换到后续端口重试。
start_api() {
    local start_port="${API_PORT}"
    local attempt=0
    local max_attempts=20
    local api_code
    API_LOG="$(mktemp -t videofeed-api.XXXXXX)"

    while ((attempt < max_attempts)); do
        if ! API_PORT="$(find_available_port "${start_port}")"; then
            echo "no available API port found near ${start_port}"
            return 1
        fi
        export SERVER_PORT="${API_PORT}"
        export VITE_API_TARGET="http://127.0.0.1:${API_PORT}"

        echo "Starting feed API"
        echo "Working directory: ${API_DIR}"
        echo "Config: ${CONFIG_FILE}"
        echo "URL: http://127.0.0.1:${API_PORT}/"
        : >"${API_LOG}"
        (cd "${API_DIR}" && exec go run ./cmd/server) >"${API_LOG}" 2>&1 &
        API_PID=$!

        sleep 1
        if kill -0 "${API_PID}" 2>/dev/null; then
            echo "API process started on ${API_PORT}"
            return 0
        fi
        if wait "${API_PID}"; then
            api_code=0
        else
            api_code=$?
        fi
        if ! grep -Eiq "listen tcp|bind:|address already in use|forbidden" "${API_LOG}"; then
            cat "${API_LOG}" >&2
            return "${api_code:-1}"
        fi

        cat "${API_LOG}" >&2
        echo "API port ${API_PORT} became unavailable; retrying"
        start_port=$((API_PORT + 1))
        attempt=$((attempt + 1))
    done
    echo "API failed after ${max_attempts} port attempts"
    return 1
}

if start_api; then
	:
else
	api_code=$?
	exit "${api_code:-1}"
fi

if [[ "${WEB_PORT}" == "${API_PORT}" ]]; then
    WEB_PORT_START=$((WEB_PORT + 1))
    if ! WEB_PORT="$(find_available_port "${WEB_PORT_START}")"; then
        echo "no available web port found near ${WEB_PORT_START}"
        exit 1
    fi
fi
export WEB_PORT

echo "Starting web dev server"
echo "Working directory: ${WEB_DIR}"
echo "URL: http://127.0.0.1:${WEB_PORT}/"
(cd "${WEB_DIR}" && exec npm run dev) &
WEB_PID=$!

echo "API PID: ${API_PID}"
echo "Web PID: ${WEB_PID}"
echo "Press Ctrl+C to stop both services"

while true; do
    if ! kill -0 "${API_PID}" 2>/dev/null; then
        wait "${API_PID}"
        exit $?
    fi
    if ! kill -0 "${WEB_PID}" 2>/dev/null; then
        wait "${WEB_PID}"
        exit $?
    fi
    sleep 1
done
