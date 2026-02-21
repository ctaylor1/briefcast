[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Version,
    [string]$ImageName = "briefcast",
    [string]$CopyTo = "\\ATLAS\docker\-builds"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptDir

if (-not $Version) {
    $latestTag = git tag --list "v[0-9]*.[0-9]*.[0-9]*" --sort=-version:refname | Select-Object -First 1
    if (-not $latestTag) {
        throw "No semantic version tags found. Pass -Version X.Y.Z."
    }

    $Version = $latestTag.Trim()
    if ($Version.StartsWith("v")) {
        $Version = $Version.Substring(1)
    }
}

if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "Invalid version '$Version'. Expected X.Y.Z."
}

$imageTag = "${ImageName}:latest"
$tarName = "${ImageName}_v$Version.tar"
$buildsDir = Join-Path $scriptDir "builds"
$tarPath = Join-Path $buildsDir $tarName

if (-not (Test-Path $buildsDir)) {
    New-Item -ItemType Directory -Path $buildsDir | Out-Null
}

if (Test-Path -LiteralPath $tarPath) {
    Write-Host "Removing existing tar: $tarPath"
    Remove-Item -LiteralPath $tarPath -Force
}

Write-Host "Building Docker image: $imageTag"
docker build -t $imageTag .
if ($LASTEXITCODE -ne 0) {
    throw "docker build failed for image tag '$imageTag'."
}

Write-Host "Saving Docker image: $tarPath"
docker image save -o $tarPath $imageTag
if ($LASTEXITCODE -ne 0) {
    throw "docker image save failed for image tag '$imageTag'."
}

if ($CopyTo) {
    $copyTarget = $CopyTo.Trim()
    if ($copyTarget -ne "") {
        $destinationPath = $copyTarget

        # If CopyTo looks like a directory, append the generated tar file name.
        if ($copyTarget.EndsWith("\") -or $copyTarget.EndsWith("/") -or -not [System.IO.Path]::HasExtension($copyTarget)) {
            if (-not (Test-Path -LiteralPath $copyTarget)) {
                New-Item -ItemType Directory -Path $copyTarget -Force | Out-Null
            }
            $destinationPath = Join-Path $copyTarget $tarName
        } else {
            $destinationDir = Split-Path -Parent $destinationPath
            if ($destinationDir -and -not (Test-Path -LiteralPath $destinationDir)) {
                New-Item -ItemType Directory -Path $destinationDir -Force | Out-Null
            }
        }

        Write-Host "Copying tar to: $destinationPath"
        Copy-Item -LiteralPath $tarPath -Destination $destinationPath -Force -ErrorAction Stop
        Write-Host "Copied: $destinationPath"
    }
}

Write-Host "Done: $tarPath"
