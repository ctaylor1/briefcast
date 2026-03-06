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

function Ensure-Directory {
    param([string]$PathValue)
    if (-not (Test-Path -LiteralPath $PathValue)) {
        New-Item -ItemType Directory -Path $PathValue -Force | Out-Null
    }
}

function Clear-DirectoryContents {
    param(
        [string]$PathValue,
        [string]$Label
    )

    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        throw "Refusing to clear empty path for $Label."
    }

    $resolved = [System.IO.Path]::GetFullPath($PathValue)
    $root = [System.IO.Path]::GetPathRoot($resolved)
    if ($resolved -eq $root) {
        throw "Refusing to clear filesystem root for ${Label}: $resolved"
    }

    Ensure-Directory -PathValue $resolved
    Get-ChildItem -LiteralPath $resolved -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
}

function Normalize-DatabaseUrlForContainer {
    param([string]$DatabaseUrl)
    if ([string]::IsNullOrWhiteSpace($DatabaseUrl)) {
        return $DatabaseUrl
    }

    $rewritten = $DatabaseUrl -replace "@localhost([:/])", "@host.docker.internal`$1"
    $rewritten = $rewritten -replace "@127\.0\.0\.1([:/])", "@host.docker.internal`$1"
    return $rewritten
}

$ProjectDir = (Resolve-Path -LiteralPath $ProjectDir).Path
$composeFile = Join-Path $ProjectDir "docker-compose.yml"
$releaseRunScript = Join-Path $ProjectDir "RELEASE_RUN.ps1"

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
if (-not (Test-Path -LiteralPath $releaseRunScript)) {
    throw "Release run script not found: $releaseRunScript"
}

$composeMode = Get-ComposeMode
$targetImage = "briefcast:latest"
if (-not $ContainerName) {
    $ContainerName = Get-EnvValue -FilePath $EnvFile -Key "BRIEFCAST_CONTAINER_NAME" -DefaultValue "briefcast"
}
if ([string]::IsNullOrWhiteSpace($ContainerName)) {
    throw "Container name cannot be empty. Set BRIEFCAST_CONTAINER_NAME in env or pass -ContainerName."
}

$databaseUrl = Get-EnvValue -FilePath $EnvFile -Key "DATABASE_URL" -DefaultValue "sqlite:///config/briefcast.db"
$dbDriver = Get-EnvValue -FilePath $EnvFile -Key "DB_DRIVER" -DefaultValue ""
if ([string]::IsNullOrWhiteSpace($dbDriver)) {
    $dbDriver = Get-EnvValue -FilePath $EnvFile -Key "DATABASE_DRIVER" -DefaultValue ""
}
if ([string]::IsNullOrWhiteSpace($dbDriver)) {
    if ($databaseUrl -match '^postgres(ql)?://') {
        $dbDriver = "postgres"
    } else {
        $dbDriver = "sqlite"
    }
}

$configDir = Resolve-ProjectPath -RootDir $ProjectDir -PathValue (Get-EnvValue -FilePath $EnvFile -Key "HOST_CONFIG_DIR" -DefaultValue "./config")
$assetsDir = Resolve-ProjectPath -RootDir $ProjectDir -PathValue (Get-EnvValue -FilePath $EnvFile -Key "HOST_ASSETS_DIR" -DefaultValue "./assets")
$logsDir = Resolve-ProjectPath -RootDir $ProjectDir -PathValue (Get-EnvValue -FilePath $EnvFile -Key "HOST_LOGS_DIR" -DefaultValue "./logs")
$backupsDir = Join-Path $configDir "backups"

Write-Warning "DESTRUCTIVE OPERATION"
Write-Host "This will reset Briefcast runtime state to a fresh-install baseline."
Write-Host "It will delete database records, downloaded assets, logs, and generated backups."
Write-Host "Config files and .env are preserved."
Write-Host ""
$confirmation = Read-Host "Type yes to continue"
if ($confirmation -ne "yes") {
    throw "Reset aborted. Confirmation was not 'yes'."
}

Write-Step "Stopping compose services"
Invoke-ComposeWithImage -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $composeFile -ProjectName $ProjectName -ImageRef $targetImage -ContainerName $ContainerName -Arguments @("down", "--remove-orphans")

if ($dbDriver -eq "postgres") {
    if ([string]::IsNullOrWhiteSpace($databaseUrl)) {
        throw "DATABASE_URL is required to reset Postgres data."
    }
    Write-Step "Resetting Postgres tables"
    $resetSql = "TRUNCATE TABLE IF EXISTS podcast_tags,podcast_items,podcasts,tags,settings,migrations,job_locks RESTART IDENTITY CASCADE;"
    $containerUrl = Normalize-DatabaseUrlForContainer -DatabaseUrl $databaseUrl
    docker run --rm --add-host host.docker.internal:host-gateway postgres:17-alpine `
        psql $containerUrl -v ON_ERROR_STOP=1 -c $resetSql | Out-Host
} else {
    Write-Step "Removing SQLite database files"
    Ensure-Directory -PathValue $configDir
    $sqliteFiles = @(
        Join-Path $configDir "briefcast.db",
        Join-Path $configDir "briefcast.db-shm",
        Join-Path $configDir "briefcast.db-wal"
    )
    foreach ($file in $sqliteFiles) {
        if (Test-Path -LiteralPath $file) {
            Remove-Item -LiteralPath $file -Force
        }
    }
}

Write-Step "Resetting backups directory"
if (Test-Path -LiteralPath $backupsDir) {
    Remove-Item -LiteralPath $backupsDir -Recurse -Force
}
Ensure-Directory -PathValue $backupsDir

Write-Step "Clearing assets directory: $assetsDir"
Clear-DirectoryContents -PathValue $assetsDir -Label "assets"

Write-Step "Clearing logs directory: $logsDir"
Clear-DirectoryContents -PathValue $logsDir -Label "logs"

Write-Step "Running standard release bring-up flow"
& $releaseRunScript -Version $Version -ProjectDir $ProjectDir -EnvFile $EnvFile -TarPath $TarPath -ProjectName $ProjectName -ContainerName $ContainerName

Write-Host "Done. Briefcast was reset and redeployed from $TarPath."
