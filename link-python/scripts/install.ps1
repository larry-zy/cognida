# Windows 开发环境安装脚本

Write-Host "安装 Python 基础服务开发环境..." -ForegroundColor Green

# 检查 uv 是否安装
if (-not (Get-Command uv -ErrorAction SilentlyContinue)) {
    Write-Host "安装 uv..." -ForegroundColor Yellow
    powershell -c "irm https://astral.sh/uv/install.ps1 | iex"
}

# 安装依赖
Write-Host "安装项目依赖..." -ForegroundColor Yellow
uv sync --all-extras

# 复制环境变量模板
if (-not (Test-Path .env)) {
    Copy-Item .env.example .env
    Write-Host "已创建 .env 文件，请根据需要修改配置" -ForegroundColor Cyan
}

Write-Host "安装完成！" -ForegroundColor Green
Write-Host "运行 'uv run uvicorn src.main:app --reload' 启动开发服务器" -ForegroundColor Cyan
