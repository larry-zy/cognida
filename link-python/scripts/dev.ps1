# Windows 开发服务器启动脚本

$env:APP_ENV = "dev"
$env:LOG_LEVEL = "DEBUG"

Write-Host "启动开发服务器..." -ForegroundColor Green
uv run uvicorn src.main:app --host 0.0.0.0 --port 8000 --reload
