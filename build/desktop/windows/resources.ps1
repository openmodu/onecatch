param(
    [switch]$Clean
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptRoot "..\..\..")).Path
$resourcePattern = Join-Path $repoRoot "cmd\app\onecatch_windows_*.syso"

if ($Clean) {
    Get-ChildItem -Path $resourcePattern -ErrorAction SilentlyContinue | Remove-Item -Force
    exit 0
}

$versionFile = Join-Path $repoRoot "VERSION"
if (-not (Test-Path -LiteralPath $versionFile -PathType Leaf)) {
    throw "VERSION file not found: $versionFile"
}
$version = if ($env:VERSION) { $env:VERSION.Trim() } else { (Get-Content -LiteralPath $versionFile -Raw).Trim() }
if ($version -notmatch '^\d+\.\d+\.\d+$') {
    throw "VERSION must use X.Y.Z numeric format, got: $version"
}

$architecture = (& go env GOARCH).Trim()
if ($LASTEXITCODE -ne 0) {
    throw "Unable to determine the Go target architecture"
}

$taskRoot = Join-Path $repoRoot ".task\windows"
New-Item -ItemType Directory -Force -Path $taskRoot | Out-Null
$infoPath = Join-Path $taskRoot "info.json"
$info = @{
    fixed = @{ file_version = $version }
    info = @{
        "0000" = @{
            ProductVersion = $version
            CompanyName = "OpenModu"
            FileDescription = "OneCatch desktop application"
            LegalCopyright = "(c) 2026, OpenModu"
            ProductName = "OneCatch"
            Comments = "Local Agent workflow orchestrator"
        }
    }
} | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText($infoPath, $info, [System.Text.UTF8Encoding]::new($false))

$outputPath = Join-Path $repoRoot "cmd\app\onecatch_windows_$architecture.syso"
$generateArguments = @(
    "tool", "wails3", "generate", "syso",
    "-arch", $architecture,
    "-icon", (Join-Path $scriptRoot "icon.ico"),
    "-manifest", (Join-Path $scriptRoot "wails.exe.manifest"),
    "-info", $infoPath,
    "-out", $outputPath
)
& go @generateArguments
if ($LASTEXITCODE -ne 0) {
    throw "Unable to generate Windows application resources"
}
