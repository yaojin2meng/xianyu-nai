# ROLLBACK.ps1 — ROLLBACK.sh 的 Windows 等价实现（本机无 sh）。
# 用法: powershell -ExecutionPolicy Bypass -File ROLLBACK.ps1 -Target <target_tree>
param(
  [Parameter(Mandatory=$true)][string]$Target
)
$ErrorActionPreference = "Stop"
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Src  = Join-Path $Here "baseline"
if (-not (Test-Path (Join-Path $Target "nai"))) { Write-Error "ROLLBACK: '$Target\nai' not found"; exit 3 }
if (-not (Test-Path (Join-Path $Src "nai")))    { Write-Error "ROLLBACK: baseline/ missing"; exit 4 }
$files = @("main.go","nai\client.go","nai\ui.go","nai\client_test.go","nai\pipeline.go","nai\sd_cmd.go")
foreach ($f in $files) {
  $s = Join-Path $Src $f
  if (-not (Test-Path $s)) { Write-Error "ROLLBACK: missing baseline file $f"; exit 5 }
  Copy-Item -LiteralPath $s -Destination (Join-Path $Target $f) -Force
  Write-Output "restored $f"
}
Write-Output "ROLLBACK_OK"
exit 0
