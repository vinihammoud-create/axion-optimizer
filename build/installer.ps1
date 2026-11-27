# Compila o instalador (dist\Axion-<versão>-setup.exe) com o Inno Setup 6, a
# partir do pacote que o build\build.ps1 monta.
#
#   .\build\build.ps1 -Version 0.10.1
#   .\build\installer.ps1 -Version 0.10.1 [-SignTool '<comando com $f>']
#
# O pacote é conferido antes: arquivos esperados, um único PresentMon (o nome
# vai para o instalador como PresentMonFile), SHA256SUMS.txt batendo com os
# executáveis e a versão do Axion.exe igual à pedida.
#
# -SignTool (desligado até haver certificado): comando de assinatura no
# formato do Inno Setup, com $f no lugar do arquivo, por exemplo
#   'signtool.exe sign /fd sha256 /tr http://timestamp.exemplo /td sha256 /f cert.pfx $f'
# O ISCC assina então o instalador e o desinstalador que vai dentro dele.
param(
  [Parameter(Mandatory = $true)][string]$Version,
  [string]$SignTool = ''
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
if (-not $iscc) { throw 'ISCC.exe não encontrado: instale o Inno Setup 6' }
"Inno Setup: $iscc"

# --- pacote ------------------------------------------------------------------
$pkg = Join-Path $root "dist\Axion-$Version-win64"
if (-not (Test-Path $pkg)) { throw "pacote não encontrado: $pkg (rode .\build\build.ps1 -Version $Version)" }
foreach ($f in 'Axion.exe', 'WebView2Loader.dll', 'LEIA-ME.txt', 'LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'MATRIZ-DE-EVIDENCIAS.md', 'SHA256SUMS.txt') {
  if (-not (Test-Path (Join-Path $pkg $f))) { throw "falta $f em $pkg" }
}
$pm = @(Get-ChildItem $pkg -File -Filter 'PresentMon-*.exe')
if ($pm.Count -ne 1) { throw "esperado um PresentMon-*.exe em $pkg, encontrados $($pm.Count)" }
$presentMon = $pm[0].Name

# Cada executável listado no SHA256SUMS.txt confere, e os três estão lá.
$listed = @{}
foreach ($line in Get-Content (Join-Path $pkg 'SHA256SUMS.txt')) {
  if ($line -match '^([0-9a-f]{64})  (.+)$') { $listed[$Matches[2]] = $Matches[1] }
}
foreach ($f in 'Axion.exe', 'WebView2Loader.dll', $presentMon) {
  if (-not $listed.ContainsKey($f)) { throw "$f não está no SHA256SUMS.txt do pacote" }
  $h = (Get-FileHash (Join-Path $pkg $f) -Algorithm SHA256).Hash.ToLower()
  if ($h -ne $listed[$f]) { throw "SHA-256 de $f não confere com o SHA256SUMS.txt do pacote" }
}
$exeVersion = (Get-Item (Join-Path $pkg 'Axion.exe')).VersionInfo.ProductVersion
if ($exeVersion -notlike "$Version*") { throw "o Axion.exe do pacote é da versão '$exeVersion', não $Version" }

# --- compilação --------------------------------------------------------------
$isccArgs = @("/DAppVersion=$Version", "/DPresentMonFile=$presentMon", "/DPackageDir=$pkg")
if ($SignTool) {
  if ($SignTool -notmatch '\$f') { throw '-SignTool precisa ter $f no lugar do arquivo a assinar' }
  $isccArgs += "/Saxionsign=$SignTool", '/DSignToolName=axionsign'
}
$setup = Join-Path $root "dist\Axion-$Version-setup.exe"
if (Test-Path $setup) { Remove-Item $setup -Force }
& $iscc @isccArgs (Join-Path $root 'installer\axion.iss')
if ($LASTEXITCODE) { throw 'ISCC falhou' }
if (-not (Test-Path $setup)) { throw "instalador não gerado: $setup" }
Get-Item $setup | ForEach-Object { "{0}  {1:N0} bytes" -f $_.Name, $_.Length }
'{0}  {1}' -f (Get-FileHash $setup -Algorithm SHA256).Hash.ToLower(), (Split-Path $setup -Leaf)
