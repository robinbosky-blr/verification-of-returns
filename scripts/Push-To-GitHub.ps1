param([string]$Target = (Join-Path (Get-Location) 'verification-of-returns'))
$ErrorActionPreference = 'Stop'
$Bundle = Join-Path $PSScriptRoot 'verification-of-returns.bundle'
if (-not (Test-Path $Bundle)) { throw 'Keep this script beside verification-of-returns.bundle from the handoff package.' }
if (Test-Path $Target) { throw "Target already exists: $Target. Choose an empty path with -Target." }
git clone --branch main $Bundle $Target
if ($LASTEXITCODE -ne 0) { throw 'Could not open the Git bundle.' }
git -C $Target remote set-url origin 'https://github.com/robinbosky-blr/verification-of-returns.git'
$RemoteHeads = git -C $Target ls-remote --heads origin
if ($LASTEXITCODE -ne 0) { throw 'GitHub access failed. Sign in through Git Credential Manager or GitHub CLI, then retry git push from the created checkout.' }
if ($RemoteHeads) { throw 'The remote repository now has commits. Review and merge before pushing; this helper never overwrites existing history.' }
git -C $Target push -u origin main
if ($LASTEXITCODE -ne 0) { throw 'Push failed. Your complete local checkout is preserved; fix GitHub write access and run git push -u origin main there.' }
Write-Host "Uploaded successfully. Source checkout: $Target"
