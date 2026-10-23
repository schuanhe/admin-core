# 仅生成缺失的本机业务密钥，绝不覆盖已有文件，也不打印令牌。
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$secretDirectory = Join-Path $projectRoot 'secrets'
$policyPath = Join-Path $secretDirectory 'service-access.json'
if (Test-Path -LiteralPath $policyPath) { Write-Output $policyPath; return }
if (-not (Test-Path -LiteralPath $secretDirectory)) { New-Item -ItemType Directory -Path $secretDirectory | Out-Null }
$bytes = New-Object byte[] 48
$generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
$policy = [ordered]@{
    'workstation-demo' = [ordered]@{
        origin = 'http://127.0.0.1:8091'
        bearer_token = [Convert]::ToBase64String($bytes)
        endpoints = @(
            @{ path = '/api/tasks'; methods = @('GET') }
            @{ path = '/api/tasks/archive'; methods = @('POST') }
            @{ path = '/api/preferences'; methods = @('GET', 'PUT') }
            @{ path = '/api/overview'; methods = @('GET') }
        )
    }
}
$json = $policy | ConvertTo-Json -Depth 8
$utf8 = New-Object System.Text.UTF8Encoding($false)
# CreateNew 防止并发运行覆盖另一进程刚生成的密钥。
$stream = [IO.File]::Open($policyPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
try { $encoded = $utf8.GetBytes($json); $stream.Write($encoded, 0, $encoded.Length) } finally { $stream.Dispose() }
Write-Output $policyPath
