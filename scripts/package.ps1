$ErrorActionPreference = "Stop"

$projectRoot = Split-Path $PSScriptRoot -Parent
$buildDirectory = Join-Path $projectRoot "build"
$releaseDirectory = Join-Path $buildDirectory "release"
$stagingDirectory = Join-Path $buildDirectory "staging"
$artifactDirectory = Join-Path $buildDirectory "artifacts"
$exePath = Join-Path $projectRoot "bin/key-crawl.exe"
$extensionDirectory = Join-Path $projectRoot "extension"
$manifestPath = Join-Path $extensionDirectory "manifest.json"

try {
    & (Join-Path $PSScriptRoot "build.ps1")
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed with exit code $LASTEXITCODE."
    }

    Remove-Item $releaseDirectory -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $stagingDirectory -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $artifactDirectory -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $artifactDirectory -Force | Out-Null

    Copy-Item $exePath (Join-Path $artifactDirectory "key-crawl.exe")
    Compress-Archive -Path (Join-Path $projectRoot "extension/*") -DestinationPath (Join-Path $artifactDirectory "extension.zip") -Force
    $extensionManifest = Get-Content (Join-Path $projectRoot "extension/manifest.json") -Raw | ConvertFrom-Json
    $extensionVersion = ([string]$extensionManifest.version).Trim()
    if ([string]::IsNullOrWhiteSpace($extensionVersion)) {
        throw "extension/manifest.json does not contain a version."
    }
    [System.IO.File]::WriteAllText((Join-Path $artifactDirectory "extension-version.txt"), $extensionVersion, [System.Text.UTF8Encoding]::new($false))
    Write-Host "Executable and update files: build/artifacts/" -ForegroundColor Green
}
catch {
    throw
}
