$ErrorActionPreference = "Stop"

$projectRoot = Split-Path $PSScriptRoot -Parent
$binDirectory = Join-Path $projectRoot "bin"
$exePath = Join-Path $binDirectory "key-crawl.exe"
$previousCgoEnabled = $env:CGO_ENABLED
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH

Push-Location $projectRoot
try {
    Write-Host "Running tests..." -ForegroundColor Cyan
    go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "Tests failed with exit code $LASTEXITCODE."
    }

    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    New-Item -ItemType Directory -Path $binDirectory -Force | Out-Null

    Write-Host "Building bin/key-crawl.exe..." -ForegroundColor Cyan
    go build -trimpath -ldflags="-s -w" -o $exePath ./src/cmd/key-crawl
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed with exit code $LASTEXITCODE."
    }

    $output = Get-Item $exePath
    Write-Host "Build complete: $($output.FullName)" -ForegroundColor Green
    Write-Host "Size: $([math]::Round($output.Length / 1MB, 2)) MB"
}
finally {
    $env:CGO_ENABLED = $previousCgoEnabled
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    Pop-Location
}
