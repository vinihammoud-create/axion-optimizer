# Headless Edge driver (Chrome DevTools Protocol over a raw WebSocket): opens the
# preview, runs optional setup JavaScript, then screenshots each hash route and
# reports the JavaScript errors the page raised.
#
#   .\build\dev\shot.ps1 -DevUrl (Get-Content $env:TEMP\axion-dev\dev.txt) -Pages painel,otimizacoes -Out C:\shots
#
# The first open of a fresh preview shows the welcome dialog, the profile picker
# and the first scan: pass -Setup with JavaScript that clicks through them (an
# async arrow function is fine, its result is printed). PowerShell 5.1 note:
# native arguments with a dot must be quoted.
param(
  [Parameter(Mandatory)][string]$DevUrl,        # http://127.0.0.1:PORT/code printed by axion-preview
  [string[]]$Pages = @('painel'),               # routes without '#/'
  [string]$Out = '.',
  [string]$Prefix = 'shot',
  [int]$W = 1440, [int]$H = 1000,
  [string[]]$Setup = @(),
  [int]$Port = 9333
)
$ErrorActionPreference = 'Stop'
$edge = "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
if (-not (Test-Path $edge)) { $edge = "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe" }
$prof = Join-Path $env:TEMP ("edge-prof-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $Out | Out-Null
$proc = Start-Process $edge -ArgumentList @('--headless=new', "--remote-debugging-port=$Port", "--user-data-dir=$prof", "--window-size=$W,$H", '--no-first-run', '--disable-gpu', 'about:blank') -PassThru
try {
  $targets = $null
  for ($i = 0; $i -lt 60 -and -not $targets; $i++) {
    try { $targets = Invoke-RestMethod "http://127.0.0.1:$Port/json" -TimeoutSec 2 } catch { Start-Sleep -Milliseconds 250 }
  }
  $page = $targets | Where-Object { $_.type -eq 'page' } | Select-Object -First 1
  $ws = New-Object System.Net.WebSockets.ClientWebSocket
  [void]$ws.ConnectAsync([uri]$page.webSocketDebuggerUrl, [Threading.CancellationToken]::None).GetAwaiter().GetResult()
  $script:id = 0

  function Recv {
    $buf = New-Object byte[] 262144
    $ms = New-Object IO.MemoryStream
    do {
      $res = $ws.ReceiveAsync([ArraySegment[byte]]::new($buf), [Threading.CancellationToken]::None).GetAwaiter().GetResult()
      $ms.Write($buf, 0, $res.Count)
    } while (-not $res.EndOfMessage)
    [Text.Encoding]::UTF8.GetString($ms.ToArray())
  }
  function Send($method, $params) {
    $script:id++; $my = $script:id
    $json = @{ id = $my; method = $method; params = $params } | ConvertTo-Json -Depth 10 -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($json)
    [void]$ws.SendAsync([ArraySegment[byte]]::new($bytes), 'Text', $true, [Threading.CancellationToken]::None).GetAwaiter().GetResult()
    while ($true) {
      $raw = Recv
      if ($raw -match '^\{"id":(\d+)' -and [int]$Matches[1] -eq $my) { return $raw }
    }
  }
  function Eval($expr) {
    $raw = Send 'Runtime.evaluate' @{ expression = $expr; returnByValue = $true; awaitPromise = $true }
    if ($raw.Length -gt 1500000) { return $null }
    $o = $raw | ConvertFrom-Json
    if ($o.result.exceptionDetails) { return 'EXC: ' + $o.result.exceptionDetails.text + ' ' + $o.result.exceptionDetails.exception.description }
    $o.result.result.value
  }
  function WaitFor($expr, $sec = 25) {
    $t = [Diagnostics.Stopwatch]::StartNew()
    while ($t.Elapsed.TotalSeconds -lt $sec) { if ((Eval $expr) -eq $true) { return $true }; Start-Sleep -Milliseconds 300 }
    $false
  }
  function Shot($name) {
    $raw = Send 'Page.captureScreenshot' @{ format = 'png' }
    $m = [regex]::Match($raw, '"data":"([^"]+)"')
    $path = Join-Path $Out ($Prefix + '-' + $name + '.png')
    [IO.File]::WriteAllBytes($path, [Convert]::FromBase64String($m.Groups[1].Value))
    $path
  }

  [void](Send 'Page.enable' @{})
  [void](Send 'Runtime.enable' @{})
  [void](Send 'Emulation.setDeviceMetricsOverride' @{ width = $W; height = $H; deviceScaleFactor = 1; mobile = $false })
  [void](Send 'Page.addScriptToEvaluateOnNewDocument' @{ source = "window.__errs=[];addEventListener('error',function(e){__errs.push(String(e.message))});addEventListener('unhandledrejection',function(e){__errs.push('rej:'+(e.reason&&e.reason.message||e.reason))});" })

  $url = (Invoke-RestMethod $DevUrl).ToString()
  [void](Send 'Page.navigate' @{ url = $url })
  [void](WaitFor "!!document.querySelector('#content') && document.querySelector('#content').children.length > 0")
  Start-Sleep -Milliseconds 800
  foreach ($js in $Setup) {
    "setup: " + (Eval $js)
    Start-Sleep -Milliseconds 1200
  }
  foreach ($p in $Pages) {
    [void](Eval "location.hash = '#/$p'")
    Start-Sleep -Milliseconds 1800
    [void](WaitFor "!document.querySelector('#content .skel')" 20)
    $path = Shot $p
    $errs = Eval 'JSON.stringify(window.__errs||[])'
    "{0}  errors={1}" -f $path, $errs
  }
} finally {
  try { $ws.Dispose() } catch {}
  if ($proc) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
  Get-CimInstance Win32_Process -Filter "Name='msedge.exe'" | Where-Object { $_.CommandLine -like "*$prof*" } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
  Start-Sleep -Milliseconds 500
  Remove-Item $prof -Recurse -Force -ErrorAction SilentlyContinue
}
