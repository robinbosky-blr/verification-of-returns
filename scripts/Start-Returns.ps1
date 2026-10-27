$ErrorActionPreference = 'Stop'
$Project = Split-Path -Parent $PSScriptRoot
Set-Location $Project
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Install Docker Desktop and start it before running this launcher.'
}
if (-not (Test-Path '.env')) {
    $SecurePassword = Read-Host 'Choose a workspace password (at least 12 characters)' -AsSecureString
    $Pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecurePassword)
    try { $AdminPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($Pointer) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Pointer) }
    if ($AdminPassword.Length -lt 12 -or $AdminPassword -match '[\r\n$#"''\\]') { throw 'Use at least 12 characters; avoid quotes, backslashes, dollar or hash characters in this launcher.' }
    $RandomBytes = New-Object byte[] 24
    $Random = [Security.Cryptography.RandomNumberGenerator]::Create()
    $Random.GetBytes($RandomBytes)
    $Random.Dispose()
    $DatabasePassword = -join ($RandomBytes | ForEach-Object { $_.ToString('x2') })
    @("POSTGRES_PASSWORD=$DatabasePassword", "ADMIN_PASSWORD=$AdminPassword", 'COOKIE_SECURE=false', 'HTTP_PORT=8088') | Set-Content -Encoding utf8 '.env'
    $AdminPassword = $null
}
docker compose up -d --build
if ($LASTEXITCODE -ne 0) { throw 'Docker startup failed. Check Docker Desktop and the displayed error.' }
Start-Process 'http://localhost:8088'
Write-Host 'ReturnScope is starting at http://localhost:8088. Sign in with your workspace password.'
