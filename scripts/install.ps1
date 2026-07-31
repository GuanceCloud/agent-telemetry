$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Binary = Join-Path (Split-Path -Parent $ScriptDir) "bin\agent-telemetry.exe"
& $Binary install @args
exit $LASTEXITCODE
