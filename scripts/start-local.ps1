param([switch]$BackendOnly)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path -LiteralPath (Join-Path $projectRoot 'config.local.json'))) {
    throw '缺少 config.local.json，请先执行 scripts/init-dev.ps1。'
}
if (-not (Test-Path -LiteralPath (Join-Path $projectRoot 'web/node_modules/vite/bin/vite.js'))) {
    throw '缺少前端依赖，请先在 web 目录执行 npm ci。'
}
$ports = @(8080, 8091)
if (-not $BackendOnly) { $ports += 5173 }
foreach ($port in $ports) {
    if (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) {
        throw "端口 $port 已被占用。请确认原进程后自行停止，脚本不会关闭已有服务。"
    }
}
$goPath = (Get-Command go.exe).Source
$commandPath = (Get-Command cmd.exe).Source
$accessFile = & (Join-Path $PSScriptRoot 'init-service-access.ps1')
$runDirectory = Join-Path $projectRoot ('.runtime/local-' + [DateTime]::Now.ToString('yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $runDirectory | Out-Null
$processes = @()
$previousOrigin = $env:CONTROLLER_PUBLIC_ORIGIN
$previousListen = $env:CONTROLLER_LISTEN
$previousCookie = $env:CONTROLLER_SECURE_COOKIE
$previousAccess = $env:CONTROLLER_SERVICE_ACCESS_FILE
try {
    # 仅对子进程设置开发来源，不改动用户配置与任何密钥。
    $env:CONTROLLER_PUBLIC_ORIGIN = 'http://127.0.0.1:5173'
    $env:CONTROLLER_LISTEN = '127.0.0.1:8080'
    $env:CONTROLLER_SECURE_COOKIE = 'false'
    $env:CONTROLLER_SERVICE_ACCESS_FILE = $accessFile
    $processes += Start-Process -FilePath $goPath -ArgumentList @('run', './cmd/controller', '-config', 'config.local.json') -WorkingDirectory $projectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runDirectory 'controller.log') -RedirectStandardError (Join-Path $runDirectory 'controller.error.log')
    $processes += Start-Process -FilePath $goPath -ArgumentList @('run', './cmd/demo-service', '-config', 'config.local.json') -WorkingDirectory $projectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runDirectory 'demo.log') -RedirectStandardError (Join-Path $runDirectory 'demo.error.log')
    if (-not $BackendOnly) {
        $processes += Start-Process -FilePath $commandPath -ArgumentList @('/d', '/c', 'npm.cmd run dev -- --port 5173 --strictPort') -WorkingDirectory (Join-Path $projectRoot 'web') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runDirectory 'frontend.log') -RedirectStandardError (Join-Path $runDirectory 'frontend.error.log')
    }
} finally {
    $env:CONTROLLER_PUBLIC_ORIGIN = $previousOrigin
    $env:CONTROLLER_LISTEN = $previousListen
    $env:CONTROLLER_SECURE_COOKIE = $previousCookie
    $env:CONTROLLER_SERVICE_ACCESS_FILE = $previousAccess
}
Write-Output '已发起 go run 与 npm run dev 本地启动，未执行打包或测试；首次启动请留意日志中的就绪或错误信息。'
Write-Output '前端：http://127.0.0.1:5173'
Write-Output '后端：http://127.0.0.1:8080'
Write-Output '独立测试服务：http://127.0.0.1:8091'
Write-Output ('进程 PID：' + (($processes | ForEach-Object Id) -join ', '))
Write-Output ('日志目录：' + $runDirectory)
