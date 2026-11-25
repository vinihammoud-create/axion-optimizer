# Compila o instalador (dist\Axion-<versão>-setup.exe) com o Inno Setup 6.
# Antes, monte o pacote: .\build\build.ps1 -Version <versão>
#
#   .\build\installer.ps1 [-Version 0.9.1]
param(
  [string]$Version = '0.9.1'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)$') { throw "versão inválida: $Version (use maior.menor.correção)" }

$iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
  $roots = @(${env:ProgramFiles(x86)}, $env:ProgramFiles, "$env:LOCALAPPDATA\Programs") | Where-Object { $_ }
  $iscc = $roots | ForEach-Object { Join-Path $_ 'Inno Setup 6\ISCC.exe' } | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $iscc) { throw 'ISCC.exe não encontrado: instale o Inno Setup 6 (choco install innosetup -y)' }

$pkg = Join-Path $root "dist\Axion-$Version-win64"
if (-not (Test-Path $pkg)) { throw "pacote não encontrado: $pkg (rode .\build\build.ps1 -Version $Version)" }
foreach ($f in 'Axion.exe', 'WebView2Loader.dll', 'PresentMon-2.6.0-x64.exe', 'LEIA-ME.txt', 'LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'MATRIZ-DE-EVIDENCIAS.md', 'SHA256SUMS.txt') {
  if (-not (Test-Path (Join-Path $pkg $f))) { throw "falta $f em $pkg" }
}

& $iscc "/DAppVersion=$Version" (Join-Path $root 'installer\axion.iss')
if ($LASTEXITCODE) { throw 'ISCC falhou' }
$setup = Join-Path $root "dist\Axion-$Version-setup.exe"
if (-not (Test-Path $setup)) { throw "instalador não gerado: $setup" }
Get-Item $setup | ForEach-Object { "{0}  {1:N0} bytes" -f $_.Name, $_.Length }
