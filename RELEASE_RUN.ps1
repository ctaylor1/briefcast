[CmdletBinding()]
param(
    [string]$Version,
    [string]$ProjectDir = $PSScriptRoot,
    [string]$EnvFile,
    [string]$TarPath,
    [string]$ProjectName,
    [string]$ContainerName
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host "==> $Message"
}

function Get-VersionFromPyProject {
    param([string]$RootDir)
    $pyprojectPath = Join-Path $RootDir "pyproject.toml"
    if (-not (Test-Path -LiteralPath $pyprojectPath)) {
        return $null
    }

    $content = Get-Content -LiteralPath $pyprojectPath -Raw
    if ($content -match '(?m)^\s*version\s*=\s*"(?<version>\d+\.\d+\.\d+)"\s*$') {
        return $Matches["version"]
    }

    return $null
}

function Get-EnvValue {
    param(
        [string]$FilePath,
        [string]$Key,
        [string]$DefaultValue = ""
    )

    if (-not (Test-Path -LiteralPath $FilePath)) {
        return $DefaultValue
    }

    foreach ($line in Get-Content -LiteralPath $FilePath) {
        $trimmed = $line.Trim()
        if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
            continue
        }
        if ($trimmed -notmatch '^[A-Za-z_][A-Za-z0-9_]*\s*=') {
            continue
        }

        $parts = $trimmed.Split("=", 2)
        if ($parts.Count -ne 2) {
            continue
        }

        if ($parts[0].Trim() -ne $Key) {
            continue
        }

        $value = $parts[1].Trim()
        if ($value.Length -ge 2) {
            if (($value.StartsWith('"') -and $value.EndsWith('"')) -or
                ($value.StartsWith("'") -and $value.EndsWith("'"))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
        }
        return $value
    }

    return $DefaultValue
}

function Resolve-ProjectPath {
    param(
        [string]$RootDir,
        [string]$PathValue
    )

    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        return $RootDir
    }

    $expanded = [Environment]::ExpandEnvironmentVariables($PathValue.Trim())
    if ($expanded -eq "~") {
        return [Environment]::GetFolderPath("UserProfile")
    }
    if ($expanded.StartsWith("~\")) {
        $tail = $expanded.Substring(2)
        return Join-Path ([Environment]::GetFolderPath("UserProfile")) $tail
    }
    if ([System.IO.Path]::IsPathRooted($expanded)) {
        return $expanded
    }
    return Join-Path $RootDir $expanded
}

function Get-ComposeMode {
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        try {
            docker compose version | Out-Null
            return "docker-compose-v2"
        } catch {
            # fall through
        }
    }

    if (Get-Command docker-compose -ErrorAction SilentlyContinue) {
        try {
            docker-compose version | Out-Null
            return "docker-compose-v1"
        } catch {
            # fall through
        }
    }

    throw "Docker Compose is not available. Install docker compose v2 or docker-compose."
}

function Invoke-Compose {
    param(
        [string]$Mode,
        [string]$EnvFilePath,
        [string]$ComposeFilePath,
        [string]$ProjectName,
        [string[]]$Arguments
    )

    $composeArgs = @("--env-file", $EnvFilePath, "-f", $ComposeFilePath)
    if (-not [string]::IsNullOrWhiteSpace($ProjectName)) {
        $composeArgs += @("-p", $ProjectName)
    }

    if ($Mode -eq "docker-compose-v2") {
        & docker compose @composeArgs @Arguments
        return
    }

    & docker-compose @composeArgs @Arguments
}

function Invoke-ComposeWithImage {
    param(
        [string]$Mode,
        [string]$EnvFilePath,
        [string]$ComposeFilePath,
        [string]$ProjectName,
        [string]$ImageRef,
        [string]$ContainerName,
        [string[]]$Arguments
    )

    $previousImage = $env:BRIEFCAST_IMAGE
    $previousContainerName = $env:BRIEFCAST_CONTAINER_NAME
    $env:BRIEFCAST_IMAGE = $ImageRef
    $env:BRIEFCAST_CONTAINER_NAME = $ContainerName
    try {
        Invoke-Compose -Mode $Mode -EnvFilePath $EnvFilePath -ComposeFilePath $ComposeFilePath -ProjectName $ProjectName -Arguments $Arguments
    } finally {
        if ($null -eq $previousImage) {
            Remove-Item Env:BRIEFCAST_IMAGE -ErrorAction SilentlyContinue
        } else {
            $env:BRIEFCAST_IMAGE = $previousImage
        }

        if ($null -eq $previousContainerName) {
            Remove-Item Env:BRIEFCAST_CONTAINER_NAME -ErrorAction SilentlyContinue
        } else {
            $env:BRIEFCAST_CONTAINER_NAME = $previousContainerName
        }
    }
}

function Ensure-WhisperEnvFile {
    param(
        [string]$RootDir,
        [string]$EnvFilePath
    )

    $whisperEnvRaw = Get-EnvValue -FilePath $EnvFilePath -Key "WHISPERX_ENV_FILE" -DefaultValue ".env.whisperx"
    $whisperEnvPath = Resolve-ProjectPath -RootDir $RootDir -PathValue $whisperEnvRaw

    $parentDir = Split-Path -Parent $whisperEnvPath
    if ($parentDir -and -not (Test-Path -LiteralPath $parentDir)) {
        New-Item -ItemType Directory -Path $parentDir -Force | Out-Null
    }

    if (-not (Test-Path -LiteralPath $whisperEnvPath)) {
        Set-Content -LiteralPath $whisperEnvPath -Value "" -NoNewline
    }
}

function Get-BriefcastContainers {
    param([string]$ContainerName)

    $lines = docker ps -a --format "{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}"
    $result = @()
    $runPattern = "$ContainerName-release-validation-*"
    foreach ($line in $lines) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }

        $parts = $line.Split("|", 4)
        if ($parts.Count -lt 4) {
            continue
        }

        $item = [PSCustomObject]@{
            Id     = $parts[0]
            Name   = $parts[1]
            Image  = $parts[2]
            Status = $parts[3]
        }

        if ($item.Name -eq $ContainerName -or
            $item.Name -like $runPattern -or
            $item.Name -like "briefcast-release-validation-*" -or
            $item.Image -like "briefcast:*" -or
            $item.Image -like "ghcr.io/ctaylor1/briefcast:*") {
            $result += $item
        }
    }

    return $result | Sort-Object Id -Unique
}

function Get-BriefcastImages {
    $lines = docker images --format "{{.Repository}}:{{.Tag}}|{{.ID}}"
    $result = @()
    foreach ($line in $lines) {
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }

        $parts = $line.Split("|", 2)
        if ($parts.Count -lt 2) {
            continue
        }

        $ref = $parts[0]
        if ($ref -like "briefcast:*" -or $ref -like "ghcr.io/ctaylor1/briefcast:*") {
            $result += [PSCustomObject]@{
                Ref = $ref
                Id  = $parts[1]
            }
        }
    }

    return $result | Sort-Object Ref -Unique
}

$ProjectDir = (Resolve-Path -LiteralPath $ProjectDir).Path
$composeFile = Join-Path $ProjectDir "docker-compose.yml"

if (-not $EnvFile) {
    $EnvFile = Join-Path $ProjectDir ".env"
}
$EnvFile = Resolve-ProjectPath -RootDir $ProjectDir -PathValue $EnvFile

if (-not $Version) {
    $Version = Get-VersionFromPyProject -RootDir $ProjectDir
}
if (-not $Version) {
    throw "Unable to resolve version from pyproject.toml. Pass -Version X.Y.Z."
}
if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "Invalid version '$Version'. Expected X.Y.Z."
}

if (-not $TarPath) {
    $TarPath = Join-Path $ProjectDir ("builds/briefcast_v{0}.tar" -f $Version)
}
$TarPath = Resolve-ProjectPath -RootDir $ProjectDir -PathValue $TarPath

if (-not (Test-Path -LiteralPath $composeFile)) {
    throw "Compose file not found: $composeFile"
}
if (-not (Test-Path -LiteralPath $EnvFile)) {
    throw "Env file not found: $EnvFile"
}
if (-not (Test-Path -LiteralPath $TarPath)) {
    throw "Release tar not found: $TarPath"
}

$composeMode = Get-ComposeMode
$targetImage = "briefcast:latest"
if (-not $ContainerName) {
    $ContainerName = Get-EnvValue -FilePath $EnvFile -Key "BRIEFCAST_CONTAINER_NAME" -DefaultValue "briefcast"
}
if ([string]::IsNullOrWhiteSpace($ContainerName)) {
    throw "Container name cannot be empty. Set BRIEFCAST_CONTAINER_NAME in env or pass -ContainerName."
}

Write-Step "Ensuring optional WhisperX env file exists"
Ensure-WhisperEnvFile -RootDir $ProjectDir -EnvFilePath $EnvFile

Write-Step "Loading release image tar: $TarPath"
docker load -i $TarPath | Out-Host

Write-Step "Stopping current compose services"
Invoke-ComposeWithImage -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $composeFile -ProjectName $ProjectName -ImageRef $targetImage -ContainerName $ContainerName -Arguments @("down", "--remove-orphans")

$containers = @(Get-BriefcastContainers -ContainerName $ContainerName)
if ($containers.Count -gt 0) {
    Write-Step "Removing old Briefcast containers"
    $containers | ForEach-Object { Write-Host (" - {0} ({1}) [{2}]" -f $_.Name, $_.Id, $_.Image) }

    foreach ($container in $containers) {
        try {
            docker rm -f $container.Id | Out-Null
        } catch {
            Write-Warning "Failed to remove container $($container.Name) ($($container.Id)): $($_.Exception.Message)"
        }
    }
} else {
    Write-Step "No old Briefcast containers to remove"
}

$images = @(Get-BriefcastImages | Where-Object { $_.Ref -ne $targetImage })
if ($images.Count -gt 0) {
    Write-Step "Removing old Briefcast images"
    $images | ForEach-Object { Write-Host (" - {0} ({1})" -f $_.Ref, $_.Id) }

    foreach ($image in $images) {
        try {
            docker rmi $image.Ref | Out-Null
        } catch {
            Write-Warning "Failed to remove image $($image.Ref): $($_.Exception.Message)"
        }
    }
} else {
    Write-Step "No old Briefcast images to remove"
}

Write-Step "Starting Briefcast with $targetImage"
Invoke-ComposeWithImage -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $composeFile -ProjectName $ProjectName -ImageRef $targetImage -ContainerName $ContainerName -Arguments @("up", "-d", "--force-recreate")

Write-Step "Current compose status"
Invoke-ComposeWithImage -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $composeFile -ProjectName $ProjectName -ImageRef $targetImage -ContainerName $ContainerName -Arguments @("ps")

if (-not [string]::IsNullOrWhiteSpace($ProjectName)) {
    Write-Host "Done. Briefcast is running with local image '$targetImage' (project '$ProjectName', container '$ContainerName')."
} else {
    Write-Host "Done. Briefcast is running with local image '$targetImage' (container '$ContainerName')."
}
