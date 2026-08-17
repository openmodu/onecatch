$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptRoot "..\..\..")).Path
$binRoot = Join-Path $repoRoot "bin"
$appBinary = Join-Path $binRoot "onecatch.exe"
$workerBinary = Join-Path $binRoot "onecatch-worker.exe"

foreach ($inputPath in @($appBinary, $workerBinary)) {
    if (-not (Test-Path -LiteralPath $inputPath -PathType Leaf)) {
        throw "Required build input not found: $inputPath"
    }
}

$version = if ($env:VERSION) { $env:VERSION } else { "0.1.0" }
$buildID = if ($env:BUILD_ID) { $env:BUILD_ID } else {
    $value = & git -C $repoRoot rev-parse --short HEAD 2>$null
    if ($LASTEXITCODE -eq 0 -and $value) { $value.Trim() } else { "dev" }
}
$architecture = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "x64" }
    "ARM64" { "arm64" }
    default { $env:PROCESSOR_ARCHITECTURE.ToLowerInvariant() }
}
$outputZip = if ($env:OUTPUT_ZIP) {
    [System.IO.Path]::GetFullPath($env:OUTPUT_ZIP, $repoRoot)
} else {
    Join-Path $binRoot "OneCatch-$version-$buildID-windows-$architecture.zip"
}

$stagingRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("onecatch-package-" + [guid]::NewGuid().ToString("N"))
$packageRoot = Join-Path $stagingRoot "OneCatch"
try {
    New-Item -ItemType Directory -Force -Path $packageRoot | Out-Null
    Copy-Item -LiteralPath $appBinary -Destination (Join-Path $packageRoot "onecatch.exe")
    Copy-Item -LiteralPath $workerBinary -Destination (Join-Path $packageRoot "onecatch-worker.exe")
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $outputZip) | Out-Null
    Compress-Archive -LiteralPath $packageRoot -DestinationPath $outputZip -CompressionLevel Optimal -Force
} finally {
    if (Test-Path -LiteralPath $stagingRoot) {
        Remove-Item -LiteralPath $stagingRoot -Recurse -Force
    }
}

$hash = Get-FileHash -LiteralPath $outputZip -Algorithm SHA256
Write-Output "Package: $outputZip"
Write-Output "Version: $version"
Write-Output "Architecture: $architecture"
Write-Output "SHA256: $($hash.Hash)"
