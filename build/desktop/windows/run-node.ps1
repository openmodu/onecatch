$ErrorActionPreference = "Stop"

$nodeCommand = Get-Command node -ErrorAction SilentlyContinue
if ($nodeCommand) {
    $nodePath = $nodeCommand.Source
} else {
    $runtimeRoot = Join-Path $env:USERPROFILE ".cache\codex-runtimes"
    $nodePath = Get-ChildItem -Path $runtimeRoot -Filter node.exe -File -Recurse -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -like "*\dependencies\node\bin\node.exe" } |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1 -ExpandProperty FullName
}

if (-not $nodePath) {
    Write-Error "Node.js was not found. Install Node.js or add node.exe to PATH."
    exit 127
}

if ($args.Count -eq 0) {
    Write-Error "No Node.js script was provided."
    exit 2
}

& $nodePath @args
exit $LASTEXITCODE
