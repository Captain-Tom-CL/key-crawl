$ErrorActionPreference = "Stop"

$projectRoot = Split-Path $PSScriptRoot -Parent
$buildDirectory = Join-Path $projectRoot "build"
$releaseDirectory = Join-Path $buildDirectory "release"
$stagingDirectory = Join-Path $buildDirectory "staging"
$packageDirectory = Join-Path $stagingDirectory "key-crawl"
$artifactDirectory = Join-Path $buildDirectory "artifacts"
$exePath = Join-Path $projectRoot "bin/key-crawl.exe"
$mainPath = Join-Path $projectRoot "src/cmd/key-crawl/main.go"

$mainSource = [System.IO.File]::ReadAllText($mainPath)
$versionPattern = [regex]::new('(?m)^const version string = "(\d+)"\r?$')
$versionMatch = $versionPattern.Match($mainSource)
if (-not $versionMatch.Success) {
    throw "Unable to find an integer version in $mainPath."
}
$version = [int64]$versionMatch.Groups[1].Value + 1
$updatedMainSource = $versionPattern.Replace($mainSource, "const version string = `"$version`"", 1)
[System.IO.File]::WriteAllText($mainPath, $updatedMainSource, [System.Text.UTF8Encoding]::new($false))
Write-Host "Version updated to $version." -ForegroundColor Cyan

try {
    & (Join-Path $PSScriptRoot "build.ps1")
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed with exit code $LASTEXITCODE."
    }

    Remove-Item $releaseDirectory -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $stagingDirectory -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $artifactDirectory -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $releaseDirectory -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $packageDirectory "bin") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $packageDirectory "data") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $packageDirectory "extension") -Force | Out-Null
    New-Item -ItemType Directory -Path $artifactDirectory -Force | Out-Null

    Copy-Item $exePath (Join-Path $packageDirectory "bin/key-crawl.exe")
    Copy-Item (Join-Path $projectRoot "data/settings.example.json") (Join-Path $packageDirectory "data/settings.json")
    Copy-Item (Join-Path $projectRoot "extension/*") (Join-Path $packageDirectory "extension") -Recurse

    [System.IO.File]::WriteAllText(
        (Join-Path $artifactDirectory "version.txt"),
        $version.ToString(),
        [System.Text.UTF8Encoding]::new($false)
    )
    Copy-Item $exePath (Join-Path $artifactDirectory "key-crawl.exe")
    Compress-Archive -Path (Join-Path $projectRoot "extension/*") -DestinationPath (Join-Path $artifactDirectory "extension.zip") -Force
    Compress-Archive -Path (Join-Path $packageDirectory "*") -DestinationPath (Join-Path $releaseDirectory "key-crawl-$version.zip") -Force
    Remove-Item $stagingDirectory -Recurse -Force

    Write-Host "Client package: build/release/key-crawl-$version.zip" -ForegroundColor Green
    Write-Host "Update files: build/artifacts/" -ForegroundColor Green
}
catch {
    [System.IO.File]::WriteAllText($mainPath, $mainSource, [System.Text.UTF8Encoding]::new($false))
    throw
}
