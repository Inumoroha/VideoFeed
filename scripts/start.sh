#!/usr/bin/env bash

# 启用严格模式：命令失败、未定义变量或管道失败时立即退出。
set -euo pipefail

# 解析脚本所在目录，并据此定位项目根目录。
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# 定义服务入口与应用启动时读取的配置文件。
SERVER_DIR="${ROOT_DIR}"
CONFIG_FILE="${ROOT_DIR}/configs/config.yaml"

# 启动前确认 Go 命令可用。
if ! command -v go >/dev/null 2>&1; then
  echo "go command is required"
  exit 1
fi

# 启动前确认默认配置文件存在。
if [[ ! -f "${CONFIG_FILE}" ]]; then
  echo "config file not found: ${CONFIG_FILE}"
  exit 1
fi

# 切换到项目根目录，确保服务可使用相对路径读取配置和 Go 模块。
cd "${SERVER_DIR}"

# 输出当前启动上下文，便于排查环境和配置问题。
echo "Starting feed API"
echo "Working directory: ${SERVER_DIR}"
echo "Config: ${CONFIG_FILE}"

# 使用当前进程启动服务，保证终止信号能直接传递给 Go 程序。
exec go run ./cmd/server
