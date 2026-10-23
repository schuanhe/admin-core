# 使用独立 Compose 项目与临时卷验证镜像，不接触正式 controller-data。
[CmdletBinding()]
param([string]$Image = 'workstation-controller:local')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$projectName = 'workstation-smoke-' + [Guid]::NewGuid().ToString('N').Substring(0, 12)
$runtimeFolder = Join-Path $projectRoot ('.runtime\' + $projectName)
$null = New-Item -ItemType Directory -Path $runtimeFolder
$envFile = Join-Path $runtimeFolder 'compose.env'
$listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()
$origin = "http://127.0.0.1:$port"
$savedEnv = @{}
Get-ChildItem Env: | Where-Object { $_.Name.StartsWith('CONTROLLER_') } | ForEach-Object {
    $savedEnv[$_.Name] = $_.Value
    Remove-Item ('Env:' + $_.Name)
}
function New-SmokeSecret {
    $bytes = New-Object byte[] 32
    $random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $random.GetBytes($bytes) } finally { $random.Dispose() }
    return [Convert]::ToBase64String($bytes)
}
$bootstrap = New-SmokeSecret
$registration = New-SmokeSecret
$password = New-SmokeSecret
$utf8 = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllLines($envFile, @(
    "CONTROLLER_BOOTSTRAP_TOKEN=$bootstrap"
    "CONTROLLER_REGISTRATION_KEY=$registration"
    "CONTROLLER_PUBLIC_ORIGIN=$origin"
    'CONTROLLER_SECURE_COOKIE=false'
    'CONTROLLER_ALLOWED_SERVICE_ORIGINS=http://127.0.0.1:8091'
    'CONTROLLER_BIND=127.0.0.1'
    "CONTROLLER_PORT=$port"
), $utf8)
$composeArgs = @('compose', '-f', (Join-Path $projectRoot 'compose.yaml'), '--project-name', $projectName, '--env-file', $envFile)
function Invoke-SmokeCompose {
    param([string[]]$Operation)
    & docker @composeArgs @Operation
    if ($LASTEXITCODE -ne 0) { throw 'Compose 操作失败' }
}
function Wait-SmokeHealth {
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        try { if ((Invoke-WebRequest -Uri "$origin/healthz" -TimeoutSec 2).StatusCode -eq 200) { return } } catch {}
        Start-Sleep -Milliseconds 500
    }
    throw '容器健康接口等待超时'
}
try {
    if ($Image -ne 'workstation-controller:local') { throw '当前Compose使用固定本地镜像，请先构建 workstation-controller:local' }
    Invoke-SmokeCompose -Operation @('up', '-d', '--no-build')
    Wait-SmokeHealth
    $status = Invoke-RestMethod -Uri "$origin/api/v1/auth/status"
    if ($status.initialized -ne $false) { throw '临时卷不是空数据库' }
    if ((Invoke-WebRequest -Uri "$origin/" -TimeoutSec 3).Content -notmatch '<div id="app">') { throw '容器没有正确托管Vue构建产物' }
    $setupBody = @{username='docker-smoke-admin'; password=$password; bootstrap_token=$bootstrap} | ConvertTo-Json
    $setup = Invoke-WebRequest -Uri "$origin/api/v1/auth/setup" -Method Post -ContentType 'application/json' -Body $setupBody
    if ($setup.StatusCode -ne 201) { throw '容器内管理员初始化失败' }
    $container = & docker @composeArgs ps -q controller
    if ($LASTEXITCODE -ne 0 -or -not $container) { throw '无法定位本次测试容器' }
    $user = & docker inspect --format '{{.Config.User}}' $container
    $readOnly = & docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' $container
    if ($user -ne '65532' -and $user -ne '65532:65532' -and $user -ne 'nonroot') { throw "容器用户不符合非root要求：$user" }
    if ($readOnly -ne 'true') { throw '容器根文件系统并非只读' }
    & docker exec $container /app/controller -healthcheck
    if ($LASTEXITCODE -ne 0) { throw '镜像内置健康检查失败' }
    Invoke-SmokeCompose -Operation @('up', '-d', '--no-build', '--force-recreate')
    Wait-SmokeHealth
    if ((Invoke-RestMethod -Uri "$origin/api/v1/auth/status").initialized -ne $true) { throw '重建容器后命名卷中的管理员丢失' }
    $result = [ordered]@{passed=$true; image=$Image; nonroot=$true; readonly_root=$true; healthcheck=$true; static_frontend=$true; recreate_persistence=$true; completed_at=[DateTime]::UtcNow.ToString('o')}
    [IO.File]::WriteAllText((Join-Path $runtimeFolder 'result.json'), ($result | ConvertTo-Json), $utf8)
    Write-Host '通过：Compose启动、Vue托管、管理员初始化、非root、只读根目录、内置健康检查、重建容器后卷持久化。'
    Write-Host "结果：$runtimeFolder\result.json"
} finally {
    # projectName由本脚本新生成，删除仅限该测试项目的容器、网络和临时数据卷。
    if ($projectName -notmatch '^workstation-smoke-[0-9a-f]{12}$') { throw '测试项目名称校验失败，禁止清理' }
    & docker @composeArgs down --volumes --remove-orphans
    Get-ChildItem Env: | Where-Object { $_.Name.StartsWith('CONTROLLER_') } | ForEach-Object { Remove-Item ('Env:' + $_.Name) }
    foreach ($entry in $savedEnv.GetEnumerator()) { Set-Item ('Env:' + $entry.Key) $entry.Value }
    Write-Host '本次临时容器和测试卷已清理；正式数据卷未受影响。'
}
