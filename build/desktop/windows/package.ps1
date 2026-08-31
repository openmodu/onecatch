$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptRoot "..\..\..")).Path
$binRoot = Join-Path $repoRoot "bin"
$versionFile = Join-Path $repoRoot "VERSION"
$appBinary = Join-Path $binRoot "onecatch.exe"
$workerBinary = Join-Path $binRoot "onecatch-worker.exe"
$askPassBinary = Join-Path $binRoot "onecatch-askpass.exe"
$updaterBinary = Join-Path $binRoot "onecatch-updater.exe"
$nsisRoot = Join-Path $scriptRoot "nsis"
$projectFile = Join-Path $nsisRoot "project.nsi"

foreach ($inputPath in @($appBinary, $workerBinary, $askPassBinary, $updaterBinary, $versionFile, $projectFile)) {
    if (-not (Test-Path -LiteralPath $inputPath -PathType Leaf)) {
        throw "Required build input not found: $inputPath"
    }
}

$version = if ($env:VERSION) { $env:VERSION.Trim() } else { (Get-Content -LiteralPath $versionFile -Raw).Trim() }
if ($version -notmatch '^\d+\.\d+\.\d+$') {
    throw "VERSION must use X.Y.Z numeric format, got: $version"
}

$goArch = (& go env GOARCH).Trim()
if ($LASTEXITCODE -ne 0) {
    throw "Unable to determine the Go target architecture"
}
$architecture = switch ($goArch) {
    "amd64" { "x64" }
    "arm64" { "arm64" }
    default { $goArch }
}

$outputInstaller = if ($env:OUTPUT_INSTALLER) {
    if ([System.IO.Path]::IsPathRooted($env:OUTPUT_INSTALLER)) {
        [System.IO.Path]::GetFullPath($env:OUTPUT_INSTALLER)
    } else {
        [System.IO.Path]::GetFullPath((Join-Path $repoRoot $env:OUTPUT_INSTALLER))
    }
} else {
    Join-Path $binRoot "OneCatch-$version-Windows-$architecture-Setup.exe"
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $outputInstaller) | Out-Null

$makeNsis = Get-Command "makensis.exe" -ErrorAction SilentlyContinue
if (-not $makeNsis) {
    $candidates = @(
        (Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe"),
        (Join-Path $env:ProgramFiles "NSIS\makensis.exe")
    )
    $makeNsisPath = $candidates |
        Where-Object { $_ -and (Test-Path -LiteralPath $_ -PathType Leaf) } |
        Select-Object -First 1
    if ($makeNsisPath) {
        $makeNsis = Get-Item -LiteralPath $makeNsisPath
    }
}
if (-not $makeNsis) {
    throw "NSIS is required. Install it with: winget install NSIS.NSIS"
}

# Wails downloads the official evergreen bootstrapper. The installer only runs
# it when WebView2 is missing on the target machine.
& go tool wails3 generate webview2bootstrapper -dir $nsisRoot
if ($LASTEXITCODE -ne 0) {
    throw "Unable to prepare the WebView2 bootstrapper"
}

$makeNsisPath = if ($makeNsis.Path) { $makeNsis.Path } else { $makeNsis.FullName }
$nsisArguments = @(
    "-WX",
    "-DAPP_VERSION=$version",
    "-DAPP_ARCH=$architecture",
    "-DAPP_BINARY=$appBinary",
    "-DWORKER_BINARY=$workerBinary",
    "-DASKPASS_BINARY=$askPassBinary",
    "-DUPDATER_BINARY=$updaterBinary",
    "-DOUTPUT_FILE=$outputInstaller",
    (Split-Path -Leaf $projectFile)
)
Push-Location $nsisRoot
try {
    & $makeNsisPath @nsisArguments
    if ($LASTEXITCODE -ne 0) {
        throw "NSIS failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

$stream = [System.IO.File]::OpenRead($outputInstaller)
$sha256 = [System.Security.Cryptography.SHA256]::Create()
try {
    $hashBytes = $sha256.ComputeHash($stream)
} finally {
    $sha256.Dispose()
    $stream.Dispose()
}
$hash = [System.BitConverter]::ToString($hashBytes).Replace("-", "").ToLowerInvariant()
$checksumFile = "$outputInstaller.sha256"
$checksumLine = "$hash  $(Split-Path -Leaf $outputInstaller)"
[System.IO.File]::WriteAllText($checksumFile, "$checksumLine`n", [System.Text.Encoding]::ASCII)

Write-Output "Installer: $outputInstaller"
Write-Output "Version: $version"
Write-Output "Architecture: $architecture"
Write-Output "Checksum: $checksumFile"
Write-Output "SHA256: $hash"
