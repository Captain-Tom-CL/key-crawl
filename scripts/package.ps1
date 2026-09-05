$ErrorActionPreference = "Stop"

$projectRoot = Split-Path $PSScriptRoot -Parent
$buildDirectory = Join-Path $projectRoot "build"
$releaseDirectory = Join-Path $buildDirectory "release"
$stagingDirectory = Join-Path $buildDirectory "staging"
$artifactDirectory = Join-Path $buildDirectory "artifacts"
$exePath = Join-Path $projectRoot "bin/key-crawl.exe"
$extensionDirectory = Join-Path $projectRoot "extension"
$manifestPath = Join-Path $extensionDirectory "manifest.json"
$originalManifestSource = $null
$manifestUpdated = $false
$extensionBaseRevision = $null

function Get-ChangedExtensionFiles {
    if ($env:GITHUB_ACTIONS -eq "true") {
        $baseRevision = $env:KEY_CRAWL_BEFORE_SHA
        if ([string]::IsNullOrWhiteSpace($baseRevision) -or $baseRevision -match "^0+$") {
            git -C $projectRoot rev-parse --verify "HEAD^" *> $null
            if ($LASTEXITCODE -eq 0) {
                $baseRevision = "HEAD^"
            }
            else {
                $changedFiles = @(git -C $projectRoot diff-tree --root --no-commit-id --name-only -r HEAD -- extension/)
                if ($LASTEXITCODE -ne 0) {
                    throw "Unable to inspect extension changes in Git."
                }
                return @($changedFiles | Where-Object { $_ } | Sort-Object -Unique)
            }
        }
        $script:extensionBaseRevision = $baseRevision
        $changedFiles = @(git -C $projectRoot diff --name-only $baseRevision HEAD -- extension/)
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to inspect extension changes between $baseRevision and HEAD."
        }
    }
    else {
        $script:extensionBaseRevision = "HEAD"
        $trackedFiles = @(git -C $projectRoot diff --name-only HEAD -- extension/)
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to inspect tracked extension changes in Git."
        }
        $untrackedFiles = @(git -C $projectRoot ls-files --others --exclude-standard -- extension/)
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to inspect untracked extension changes in Git."
        }
        $changedFiles = @($trackedFiles) + @($untrackedFiles)
    }

    return @($changedFiles | Where-Object { $_ } | Sort-Object -Unique)
}

function Update-ExtensionVersion {
    param([string[]]$ChangedFiles)

    if ($ChangedFiles.Count -eq 0) {
        return
    }

    $script:originalManifestSource = [System.IO.File]::ReadAllText($manifestPath)
    $versionPattern = [regex]::new('("version"\s*:\s*")(\d+(?:\.\d+){0,3})(")')
    $versionMatch = $versionPattern.Match($script:originalManifestSource)
    if (-not $versionMatch.Success) {
        throw "Unable to find a valid Chrome extension version in $manifestPath."
    }
    if (($ChangedFiles -contains "extension/manifest.json") -and $script:extensionBaseRevision) {
        $baseManifestSource = (& git -C $projectRoot show "${script:extensionBaseRevision}:extension/manifest.json" 2>$null | Out-String)
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to read extension/manifest.json from $script:extensionBaseRevision."
        }
        $baseVersionMatch = $versionPattern.Match($baseManifestSource)
        if (-not $baseVersionMatch.Success) {
            throw "Unable to read the base Chrome extension version."
        }
        if ($versionMatch.Groups[2].Value -ne $baseVersionMatch.Groups[2].Value) {
            Write-Host "Extension version is already updated to $($versionMatch.Groups[2].Value)." -ForegroundColor DarkGray
            return
        }
    }

    [int[]]$parts = $versionMatch.Groups[2].Value.Split(".")
    for ($index = $parts.Length - 1; $index -ge 0; $index--) {
        if ($parts[$index] -lt 65535) {
            $parts[$index]++
            for ($resetIndex = $index + 1; $resetIndex -lt $parts.Length; $resetIndex++) {
                $parts[$resetIndex] = 0
            }
            break
        }
        if ($index -eq 0) {
            throw "Chrome extension version has reached its maximum value."
        }
    }

    $newVersion = $parts -join "."
    $updatedManifestSource = $versionPattern.Replace(
        $script:originalManifestSource,
        { param($match) $match.Groups[1].Value + $newVersion + $match.Groups[3].Value },
        1
    )
    [System.IO.File]::WriteAllText($manifestPath, $updatedManifestSource, [System.Text.UTF8Encoding]::new($false))
    $script:manifestUpdated = $true
    Write-Host "Extension changes detected; version updated to $newVersion." -ForegroundColor Cyan
    Write-Host "Changed extension files: $($ChangedFiles -join ', ')" -ForegroundColor DarkGray
}

try {
    Update-ExtensionVersion -ChangedFiles @(Get-ChangedExtensionFiles)

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
    if ($manifestUpdated -and $null -ne $originalManifestSource) {
        [System.IO.File]::WriteAllText($manifestPath, $originalManifestSource, [System.Text.UTF8Encoding]::new($false))
    }
    throw
}
