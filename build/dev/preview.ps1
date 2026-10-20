# Rebuilds the preview server (the interface is embedded in the exe, so every
# edit of internal/web/static needs a rebuild), restarts it and prints the URL
# that hands out a fresh one-time link. The preview runs the real interface over
# the simulated machine: nothing real is read or changed.
#
#   .\build\dev\preview.ps1 [-OutDir C:\somewhere]
param([string]$OutDir = (Join-Path $env:TEMP 'axion-dev'))
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
New-Item -ItemType Directory -Force $OutDir | Out-Null
$goDir = 'C:\Program Files\Go\bin'
if (Test-Path $goDir) { $env:Path = "$goDir;$env:Path" }
$env:GOFLAGS = '-mod=mod'; $env:GOPROXY = 'off'; $env:GOSUMDB = 'off'; $env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
Get-Process axion-preview -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 400
Push-Location $root
try { go build -o (Join-Path $OutDir 'axion-preview.exe') ./cmd/axion-preview; if ($LASTEXITCODE) { throw 'build failed' } } finally { Pop-Location }
$out = Join-Path $OutDir 'preview.out'
[void](Start-Process (Join-Path $OutDir 'axion-preview.exe') -RedirectStandardOutput $out -RedirectStandardError (Join-Path $OutDir 'preview.err') -PassThru -WindowStyle Hidden)
for ($i = 0; $i -lt 40; $i++) {
  Start-Sleep -Milliseconds 250
  if ((Test-Path $out) -and (Get-Content $out -ErrorAction SilentlyContinue | Where-Object { $_ -like 'dev=*' })) { break }
}
$dev = (Get-Content $out | Where-Object { $_ -like 'dev=*' } | Select-Object -First 1) -replace '^dev=', ''
Set-Content (Join-Path $OutDir 'dev.txt') $dev -Encoding ascii
$dev
