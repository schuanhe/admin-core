# 生成本机开发配置，不覆盖已有配置，不将随机密钥打印到终端。
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $projectRoot 'config.local.json'
$envPath = Join-Path $projectRoot '.env'
if ((Test-Path -LiteralPath $configPath) -or (Test-Path -LiteralPath $envPath)) {
    throw 'config.local.json 或 .env 已存在。为保护配置，本脚本不会覆盖；请人工检查现有文件。'
}
function New-LocalSecret {
    $bytes = New-Object byte[] 48
    $generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    return [Convert]::ToBase64String($bytes)
}
$bootstrap = New-LocalSecret
$registration = New-LocalSecret
$config = [ordered]@{
    listen = '127.0.0.1:8080'
    database_path = 'data/controller.db'
    static_dir = 'web/dist'
    public_origin = 'http://localhost:8080'
    bootstrap_token = $bootstrap
    registration_key = $registration
    allowed_service_origins = @('http://127.0.0.1:8091')
    session_hours = 12
    secure_cookie = $false
    check_interval_seconds = 30
    service_access_file = 'secrets/service-access.json'
}
$utf8 = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 5), $utf8)
$lines = @(
    "CONTROLLER_BOOTSTRAP_TOKEN=$bootstrap"
    "CONTROLLER_REGISTRATION_KEY=$registration"
    'CONTROLLER_PUBLIC_ORIGIN=http://localhost:8080'
    'CONTROLLER_SECURE_COOKIE=false'
    'CONTROLLER_ALLOWED_SERVICE_ORIGINS=http://127.0.0.1:8091'
    'CONTROLLER_SESSION_HOURS=12'
    'CONTROLLER_CHECK_INTERVAL_SECONDS=30'
    'CONTROLLER_SERVICE_ACCESS_FILE=/run/secrets/service-access.json'
    'CONTROLLER_BIND=127.0.0.1'
    'CONTROLLER_PORT=8080'
)
[IO.File]::WriteAllLines($envPath, $lines, $utf8)
& (Join-Path $PSScriptRoot 'init-service-access.ps1') | Out-Null
Write-Host '已生成 config.local.json 与 .env（均已被 Git 和镜像构建忽略）。'
Write-Host '初始化页面需要 config.local.json 中的 bootstrap_token；请仅在本机打开该文件查看。'
Write-Host '这些文件含本地密钥，请限制文件访问权限，不要共享或提交。'
