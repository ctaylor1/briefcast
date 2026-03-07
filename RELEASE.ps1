<#
.SYNOPSIS
    Unified release pipeline: test, build, publish, deploy, verify, rollback, reset, ship.

.DESCRIPTION
    A single-file, project-agnostic release script driven by release.config.json.
    Run without arguments for help.

.PARAMETER Stage
    Pipeline stage to execute: test, build, publish, deploy, verify, rollback, reset, ship.

.PARAMETER Bump
    Version bump type: major, minor, or patch. Used by publish and ship stages.

.PARAMETER Version
    Explicit version to set (X.Y.Z). Alternative to -Bump.

.PARAMETER EnvFile
    Path to .env file for deploy/compose operations. Default from config.

.PARAMETER ComposeFile
    Path to docker-compose.yml. Default from config.

.PARAMETER BuildDir
    Directory for build artifacts. Default: builds/

.PARAMETER Remote
    Git remote name. Default: origin.

.PARAMETER SkipTests
    Skip test stage in ship pipeline.

.PARAMETER SkipSmoke
    Skip Docker smoke tests during build.

.PARAMETER SkipVerify
    Skip verify stage after deploy.

.PARAMETER NoCache
    Build Docker images with --no-cache.

.PARAMETER NoPush
    Skip git push in publish stage.

.PARAMETER Force
    Skip confirmation prompts (e.g. reset).

.EXAMPLE
    .\RELEASE.ps1
    # Shows help text with all stages and options.

.EXAMPLE
    .\RELEASE.ps1 test
    # Runs all quality checks defined in release.config.json.

.EXAMPLE
    .\RELEASE.ps1 ship -Bump patch
    # Full pipeline: test -> bump -> build -> commit+tag+push -> deploy -> verify.

.EXAMPLE
    .\RELEASE.ps1 build -Version 2.0.0
    # Builds Docker images for an explicit version.

.EXAMPLE
    .\RELEASE.ps1 deploy
    # Loads .tar artifacts, deploys via compose, verifies health.

.EXAMPLE
    .\RELEASE.ps1 rollback
    # Restores previous deployment from rollback-state.json.

.EXAMPLE
    .\RELEASE.ps1 reset -Force
    # Tears down stack, wipes volumes and data dirs, rebuilds.
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet("test", "build", "publish", "deploy", "verify", "rollback", "reset", "ship", "")]
    [string]$Stage = "",

    [ValidateSet("major", "minor", "patch", "")]
    [string]$Bump = "",

    [string]$Version = "",
    [string]$EnvFile = "",
    [string]$ComposeFile = "",
    [string]$BuildDir = "",
    [string]$Remote = "origin",

    [switch]$SkipTests,
    [switch]$SkipSmoke,
    [switch]$SkipVerify,
    [switch]$NoCache,
    [switch]$NoPush,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ScriptRoot = $PSScriptRoot
if (-not $ScriptRoot) { $ScriptRoot = (Get-Location).Path }

# ═══════════════════════════════════════════════════════════════════════════════
# HELPERS — defined exactly once
# ═══════════════════════════════════════════════════════════════════════════════

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Detail {
    param([string]$Message)
    Write-Host "    $Message" -ForegroundColor Gray
}

function Write-Ok {
    param([string]$Message)
    Write-Host "    [OK] $Message" -ForegroundColor Green
}

function Write-Skip {
    param([string]$Message)
    Write-Host "    [SKIP] $Message" -ForegroundColor Yellow
}

function Write-Fail {
    param([string]$Message)
    Write-Host "    [FAIL] $Message" -ForegroundColor Red
}

function Get-ElapsedString {
    param([datetime]$Start)
    $elapsed = (Get-Date) - $Start
    if ($elapsed.TotalMinutes -ge 1) {
        return "{0:N1} min" -f $elapsed.TotalMinutes
    }
    return "{0:N0}s" -f $elapsed.TotalSeconds
}

function Invoke-Step {
    param(
        [string]$Label,
        [string]$Command,
        [string]$WorkDir
    )
    Write-Detail "Running: $Label"
    $prevDir = Get-Location
    $prevErrorAction = $ErrorActionPreference
    try {
        Set-Location $WorkDir
        # Temporarily allow stderr output without terminating (tools like uv write warnings to stderr)
        $ErrorActionPreference = "Continue"
        Invoke-Expression $Command 2>&1 | Out-Host
        $ErrorActionPreference = $prevErrorAction
        if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
            throw "$Label failed (exit code $LASTEXITCODE)."
        }
        Write-Ok $Label
    } finally {
        $ErrorActionPreference = $prevErrorAction
        Set-Location $prevDir
    }
}

# ─── .env reader ──────────────────────────────────────────────────────────────

function Read-DotEnv {
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
        if ($trimmed -eq "" -or $trimmed.StartsWith("#")) { continue }
        if ($trimmed -notmatch '^[A-Za-z_][A-Za-z0-9_]*\s*=') { continue }

        $parts = $trimmed.Split("=", 2)
        if ($parts.Count -ne 2) { continue }
        if ($parts[0].Trim() -ne $Key) { continue }

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

# ─── Path resolver ────────────────────────────────────────────────────────────

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
    if ($expanded.StartsWith("~\") -or $expanded.StartsWith("~/")) {
        $tail = $expanded.Substring(2)
        return Join-Path ([Environment]::GetFolderPath("UserProfile")) $tail
    }
    if ([System.IO.Path]::IsPathRooted($expanded)) {
        return $expanded
    }
    return Join-Path $RootDir $expanded
}

# ─── Version read/write — auto-detects by filename ────────────────────────────

function Read-VersionFile {
    param([string]$FilePath)
    $content = Get-Content -LiteralPath $FilePath -Raw
    $name = Split-Path -Leaf $FilePath

    switch -Wildcard ($name) {
        "pyproject.toml" {
            if ($content -match '(?m)^\s*version\s*=\s*"(?<ver>\d+\.\d+\.\d+)"') {
                return $Matches["ver"]
            }
        }
        "package.json" {
            if ($content -match '"version"\s*:\s*"(?<ver>\d+\.\d+\.\d+)"') {
                return $Matches["ver"]
            }
        }
        "Cargo.toml" {
            if ($content -match '(?m)^\s*version\s*=\s*"(?<ver>\d+\.\d+\.\d+)"') {
                return $Matches["ver"]
            }
        }
        "VERSION*" {
            $ver = $content.Trim()
            if ($ver -match '^\d+\.\d+\.\d+$') {
                return $ver
            }
        }
    }

    throw "Could not parse version from $FilePath"
}

function Write-VersionFile {
    param(
        [string]$FilePath,
        [string]$OldVersion,
        [string]$NewVersion
    )

    $content = Get-Content -LiteralPath $FilePath -Raw
    $name = Split-Path -Leaf $FilePath

    $pattern = switch -Wildcard ($name) {
        "pyproject.toml" { '(version\s*=\s*")[\d]+\.[\d]+\.[\d]+(")' }
        "package.json"   { '("version"\s*:\s*")[\d]+\.[\d]+\.[\d]+(")' }
        "Cargo.toml"     { '(version\s*=\s*")[\d]+\.[\d]+\.[\d]+(")' }
        "VERSION*"       { '(^)[\d]+\.[\d]+\.[\d]+($)' }
        default          { throw "Unsupported version file format: $name" }
    }

    if ($OldVersion -eq $NewVersion) {
        Write-Detail "$FilePath`: already at $NewVersion"
        return
    }
    $replaced = $content -replace $pattern, "`${1}$NewVersion`${2}"
    if ($replaced -eq $content) {
        throw "Failed to update version in $FilePath - pattern did not match."
    }
    Set-Content -LiteralPath $FilePath -Value $replaced -NoNewline
    Write-Detail "$FilePath`: $OldVersion -> $NewVersion"
}

function Get-BumpedVersion {
    param(
        [string]$CurrentVersion,
        [string]$Type
    )
    $parts = $CurrentVersion.Split(".")
    $major = [int]$parts[0]; $minor = [int]$parts[1]; $patch = [int]$parts[2]

    switch ($Type) {
        "major" { $major++; $minor = 0; $patch = 0 }
        "minor" { $minor++; $patch = 0 }
        "patch" { $patch++ }
    }

    return "$major.$minor.$patch"
}

# ─── Docker Compose wrapper ───────────────────────────────────────────────────

function Get-ComposeMode {
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        try { docker compose version 2>&1 | Out-Null; return "v2" } catch { }
    }
    if (Get-Command docker-compose -ErrorAction SilentlyContinue) {
        try { docker-compose version 2>&1 | Out-Null; return "v1" } catch { }
    }
    throw "Docker Compose is not available. Install docker compose v2 or docker-compose."
}

function Invoke-Compose {
    param(
        [string]$Mode,
        [string]$EnvFilePath,
        [string]$ComposeFilePath,
        [string]$ProjectName,
        [hashtable]$EnvOverrides = @{},
        [string[]]$Arguments
    )

    # Set env overrides
    $savedEnv = @{}
    foreach ($key in $EnvOverrides.Keys) {
        $savedEnv[$key] = [Environment]::GetEnvironmentVariable($key)
        [Environment]::SetEnvironmentVariable($key, $EnvOverrides[$key])
    }

    try {
        $composeArgs = @("--env-file", $EnvFilePath, "-f", $ComposeFilePath)
        if (-not [string]::IsNullOrWhiteSpace($ProjectName)) {
            $composeArgs += @("-p", $ProjectName)
        }

        $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
        if ($Mode -eq "v2") {
            & docker compose @composeArgs @Arguments 2>&1 | Out-Host
        } else {
            & docker-compose @composeArgs @Arguments 2>&1 | Out-Host
        }
        $ErrorActionPreference = $oldEA
    } finally {
        foreach ($key in $savedEnv.Keys) {
            if ($null -eq $savedEnv[$key]) {
                [Environment]::SetEnvironmentVariable($key, $null)
            } else {
                [Environment]::SetEnvironmentVariable($key, $savedEnv[$key])
            }
        }
    }
}

# ─── Directory helpers ────────────────────────────────────────────────────────

function Ensure-Directory {
    param([string]$PathValue)
    if (-not (Test-Path -LiteralPath $PathValue)) {
        New-Item -ItemType Directory -Path $PathValue -Force | Out-Null
    }
}

function Clear-DirectoryContents {
    param([string]$PathValue, [string]$Label)

    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        throw "Refusing to clear empty path for $Label."
    }
    $resolved = [System.IO.Path]::GetFullPath($PathValue)
    if ($resolved -eq [System.IO.Path]::GetPathRoot($resolved)) {
        throw "Refusing to clear filesystem root for ${Label}: $resolved"
    }
    Ensure-Directory -PathValue $resolved
    Get-ChildItem -LiteralPath $resolved -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
}

# ─── Checksum helper ─────────────────────────────────────────────────────────

function Write-ChecksumsFile {
    param([string]$Directory)
    $checksumPath = Join-Path $Directory "checksums.sha256"
    $lines = @()
    foreach ($file in Get-ChildItem -LiteralPath $Directory -Filter "*.tar" -File) {
        $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLower()
        $lines += "$hash  $($file.Name)"
    }
    if ($lines.Count -gt 0) {
        Set-Content -LiteralPath $checksumPath -Value ($lines -join "`n") -NoNewline
        Write-Ok "Checksums written to $checksumPath"
    }
}

# ─── WhisperX env file helper (from existing scripts) ────────────────────────

function Ensure-WhisperEnvFile {
    param([string]$RootDir, [string]$EnvFilePath, $Config)
    $whisperKey = "WHISPERX_ENV_FILE"
    $whisperDefault = ".env.whisperx"
    if ($Config.deploy.whisperx_env_file_key) { $whisperKey = $Config.deploy.whisperx_env_file_key }
    if ($Config.deploy.whisperx_env_file_default) { $whisperDefault = $Config.deploy.whisperx_env_file_default }

    $whisperEnvRaw = Read-DotEnv -FilePath $EnvFilePath -Key $whisperKey -DefaultValue $whisperDefault
    $whisperEnvPath = Resolve-ProjectPath -RootDir $RootDir -PathValue $whisperEnvRaw
    $parentDir = Split-Path -Parent $whisperEnvPath
    if ($parentDir -and -not (Test-Path -LiteralPath $parentDir)) {
        New-Item -ItemType Directory -Path $parentDir -Force | Out-Null
    }
    if (-not (Test-Path -LiteralPath $whisperEnvPath)) {
        Set-Content -LiteralPath $whisperEnvPath -Value "" -NoNewline
    }
}

# ═══════════════════════════════════════════════════════════════════════════════
# LOAD CONFIG
# ═══════════════════════════════════════════════════════════════════════════════

$configPath = Join-Path $ScriptRoot "release.config.json"
if (-not (Test-Path -LiteralPath $configPath)) {
    Write-Host "release.config.json not found in $ScriptRoot" -ForegroundColor Red
    exit 1
}
$Config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json

# Resolve defaults from config
$projectName = $Config.project
if (-not $BuildDir) { $BuildDir = Join-Path $ScriptRoot "builds" }
if (-not $ComposeFile) {
    $cf = $Config.deploy.compose_file
    if (-not $cf) { $cf = "docker-compose.yml" }
    $ComposeFile = Join-Path $ScriptRoot $cf
}
if (-not $EnvFile) {
    $ef = $Config.deploy.env_file
    if (-not $ef) { $ef = ".env" }
    $EnvFile = Join-Path $ScriptRoot $ef
}

$versionFilePath = Join-Path $ScriptRoot $Config.version.file

# ═══════════════════════════════════════════════════════════════════════════════
# HELP TEXT
# ═══════════════════════════════════════════════════════════════════════════════

function Show-Help {
    $help = @'

RELEASE.ps1 - Unified release pipeline

STAGES
  test       Run all quality checks: lint, typecheck, unit tests
  build      Build Docker images, smoke test, export .tar + checksums
  publish    Bump version, commit, tag, push to remote
  deploy     Load .tar, save rollback state, docker compose up, verify
  verify     Check services are running + healthy, poll HTTP endpoints
  rollback   Restore previous deployment from rollback-state.json
  reset      Tear down stack, wipe volumes + data dirs, rebuild [destructive]
  ship       Full pipeline: test > bump > build > commit+tag+push > deploy > verify

OPTIONS
  -Bump [major|minor|patch]   Version bump type [publish, ship]
  -Version [X.Y.Z]            Explicit version [alternative to -Bump]
  -EnvFile [path]             Path to .env file         [default: from config]
  -ComposeFile [path]         Path to compose file      [default: from config]
  -BuildDir [path]            Artifact output directory [default: builds/]
  -Remote [name]              Git remote name           [default: origin]
  -SkipTests                  Skip test stage in ship
  -SkipSmoke                  Skip Docker smoke tests
  -SkipVerify                 Skip verify after deploy
  -NoCache                    Docker build --no-cache
  -NoPush                     Skip git push in publish
  -Force                      Skip confirmation prompts

EXAMPLES
  .\RELEASE.ps1 test                        # Run all quality checks
  .\RELEASE.ps1 build -Version 2.0.0        # Build images for explicit version
  .\RELEASE.ps1 ship -Bump patch            # Full release pipeline
  .\RELEASE.ps1 deploy                      # Deploy latest build
  .\RELEASE.ps1 rollback                    # Restore previous deployment
  .\RELEASE.ps1 reset -Force                # Full teardown + rebuild
  .\RELEASE.ps1 publish -Bump minor -NoPush # Bump + commit + tag, no push
'@
    Write-Host "RELEASE.ps1 - Unified release pipeline" -ForegroundColor Cyan
    Write-Host "Driven by release.config.json. Project: $projectName" -ForegroundColor Gray
    Write-Host $help
    Write-Host ""
}

if (-not $Stage) {
    Show-Help
    exit 0
}

# ═══════════════════════════════════════════════════════════════════════════════
# RESOLVE VERSION (used by build, publish, ship)
# ═══════════════════════════════════════════════════════════════════════════════

function Resolve-TargetVersion {
    if ($Version) {
        if ($Version -notmatch '^\d+\.\d+\.\d+$') {
            throw "Invalid version '$Version'. Expected X.Y.Z."
        }
        return $Version
    }
    if ($Bump) {
        $current = Read-VersionFile -FilePath $versionFilePath
        return Get-BumpedVersion -CurrentVersion $current -Type $Bump
    }
    # Fall back to reading current version from file
    return Read-VersionFile -FilePath $versionFilePath
}

function Resolve-ContainerName {
    $cnEnv = $Config.deploy.container_name_env
    if (-not $cnEnv) { $cnEnv = "BRIEFCAST_CONTAINER_NAME" }
    $cnDefault = $Config.deploy.container_name_default
    if (-not $cnDefault) { $cnDefault = $projectName }
    if (Test-Path -LiteralPath $EnvFile) {
        return Read-DotEnv -FilePath $EnvFile -Key $cnEnv -DefaultValue $cnDefault
    }
    return $cnDefault
}

function Resolve-EnvOverrides {
    param([string]$ImageRef, [string]$ContainerName)
    $overrides = @{}
    if ($Config.deploy.env_overrides) {
        foreach ($prop in $Config.deploy.env_overrides.PSObject.Properties) {
            $val = $prop.Value -replace '\{image\}', $ImageRef
            $val = $val -replace '\{container_name\}', $ContainerName
            $overrides[$prop.Name] = $val
        }
    }
    return $overrides
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: TEST
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageTest {
    Write-Step "Running quality checks"
    $testStart = Get-Date

    foreach ($test in $Config.test) {
        $dir = Resolve-ProjectPath -RootDir $ScriptRoot -PathValue $test.dir
        Invoke-Step -Label $test.name -Command $test.run -WorkDir $dir
    }

    $elapsed = Get-ElapsedString -Start $testStart
    Write-Ok "All quality checks passed ($elapsed)"
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: BUILD
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageBuild {
    param([string]$TargetVersion)
    Write-Step "Building Docker images (v$TargetVersion)"
    $buildStart = Get-Date

    Ensure-Directory -PathValue $BuildDir

    # Pre-flight: Docker available
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "Docker is not installed or not in PATH."
    }
    $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
    docker info > $null 2>&1
    $ErrorActionPreference = $oldEA
    if ($LASTEXITCODE -ne 0) { throw "Docker daemon is not running." }

    foreach ($img in $Config.images) {
        $imageName = $img.name
        $dockerfile = Join-Path $ScriptRoot $img.dockerfile
        $context = Resolve-ProjectPath -RootDir $ScriptRoot -PathValue $img.context
        $imageTag = "${imageName}:${TargetVersion}"
        $imageLatest = "${imageName}:latest"

        Write-Detail "Building $imageName..."
        $dockerArgs = @("build", "-f", $dockerfile, "-t", $imageLatest, "-t", $imageTag)

        if ($NoCache) { $dockerArgs += "--no-cache" }

        # Build args from config
        if ($img.build_args) {
            foreach ($prop in $img.build_args.PSObject.Properties) {
                $val = $prop.Value -replace '\{version\}', $TargetVersion
                $dockerArgs += @("--build-arg", "$($prop.Name)=$val")
            }
        }

        $dockerArgs += $context

        $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
        & docker @dockerArgs 2>&1 | Out-Host
        $ErrorActionPreference = $oldEA
        if ($LASTEXITCODE -ne 0) { throw "Docker build failed for $imageName." }
        Write-Ok "Built: $imageLatest, $imageTag"

        # Smoke test
        if (-not $SkipSmoke -and $img.smoke_test) {
            $smokeCmd = $img.smoke_test -replace '\{image\}', $imageLatest
            Write-Detail "Smoke test: $smokeCmd"
            $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
            Invoke-Expression $smokeCmd 2>&1 | Out-Host
            $ErrorActionPreference = $oldEA
            if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
                throw "Smoke test failed for $imageName."
            }
            Write-Ok "Smoke test passed"
        }

        # Export tar
        $tarName = "${imageName}_v${TargetVersion}.tar"
        $tarPath = Join-Path $BuildDir $tarName
        if (Test-Path -LiteralPath $tarPath) {
            Remove-Item -LiteralPath $tarPath -Force
        }

        Write-Detail "Exporting $tarPath..."
        $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
        docker save -o $tarPath $imageLatest 2>&1 | Out-Host
        $ErrorActionPreference = $oldEA
        if ($LASTEXITCODE -ne 0) { throw "Docker save failed for $imageName." }

        $tarSizeMB = [math]::Round((Get-Item -LiteralPath $tarPath).Length / 1MB, 1)
        Write-Ok "Exported: $tarPath ($tarSizeMB MB)"
    }

    Write-ChecksumsFile -Directory $BuildDir

    $elapsed = Get-ElapsedString -Start $buildStart
    Write-Ok "Build complete ($elapsed)"
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: PUBLISH
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StagePublish {
    param([string]$TargetVersion)
    Write-Step "Publishing v$TargetVersion"

    $tagName = "v$TargetVersion"

    # Check tag doesn't already exist
    Push-Location $ScriptRoot
    try {
        $existingTag = git tag -l $tagName 2>$null
    } finally { Pop-Location }
    if ($existingTag) {
        throw "Tag '$tagName' already exists."
    }

    # Bump version in all version files
    Write-Step "Updating version files"
    $currentVersion = Read-VersionFile -FilePath $versionFilePath
    Write-VersionFile -FilePath $versionFilePath -OldVersion $currentVersion -NewVersion $TargetVersion

    if ($Config.version.sync) {
        foreach ($syncFile in $Config.version.sync) {
            $syncPath = Join-Path $ScriptRoot $syncFile
            if (Test-Path -LiteralPath $syncPath) {
                Write-VersionFile -FilePath $syncPath -OldVersion $currentVersion -NewVersion $TargetVersion
            } else {
                Write-Detail "Sync file not found, skipping: $syncPath"
            }
        }
    }

    # Git commit and tag
    Write-Step "Committing release"
    Push-Location $ScriptRoot
    try {
        $commitMsg = "release: v$TargetVersion"
        $addArgs = @("add", "-A", "--", ".")
        if ($Config.git -and $Config.git.exclude_paths) {
            foreach ($excl in $Config.git.exclude_paths) {
                $addArgs += ":(exclude)$excl"
            }
        }

        & git @addArgs
        # Remove any cached worktree submodules
        if ($Config.git -and $Config.git.exclude_paths) {
            foreach ($excl in $Config.git.exclude_paths) {
                git rm --cached -r --ignore-unmatch $excl 2>$null | Out-Null
            }
        }

        git commit -m $commitMsg
        if ($LASTEXITCODE -ne 0) {
            # Retry after pre-commit hooks may have modified files
            Write-Detail "Retrying commit after pre-commit hooks..."
            & git @addArgs
            git commit -m $commitMsg
            if ($LASTEXITCODE -ne 0) {
                throw "Git commit failed after retry."
            }
        }
        Write-Ok "Committed: $commitMsg"

        git tag $tagName
        if ($LASTEXITCODE -ne 0) { throw "Git tag failed." }
        Write-Ok "Tagged: $tagName"

        if (-not $NoPush) {
            Write-Detail "Pushing to $Remote..."
            git push $Remote main --tags
            if ($LASTEXITCODE -ne 0) { throw "Git push failed." }
            Write-Ok "Pushed to $Remote"
        } else {
            Write-Skip "Git push (--NoPush)"
        }
    } finally { Pop-Location }
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: DEPLOY
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageDeploy {
    param([string]$TargetVersion)
    Write-Step "Deploying v$TargetVersion"

    if (-not (Test-Path -LiteralPath $EnvFile)) {
        throw "Env file not found: $EnvFile. Cannot deploy without it."
    }
    if (-not (Test-Path -LiteralPath $ComposeFile)) {
        throw "Compose file not found: $ComposeFile"
    }

    $composeMode = Get-ComposeMode
    $containerName = Resolve-ContainerName

    # Ensure optional WhisperX env file exists
    Ensure-WhisperEnvFile -RootDir $ScriptRoot -EnvFilePath $EnvFile -Config $Config

    # Load tar artifacts
    foreach ($img in $Config.images) {
        $tarName = "$($img.name)_v${TargetVersion}.tar"
        $tarPath = Join-Path $BuildDir $tarName
        if (-not (Test-Path -LiteralPath $tarPath)) {
            throw "Release tar not found: $tarPath. Run 'build' stage first."
        }
        Write-Detail "Loading $tarPath..."
        $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
        docker load -i $tarPath 2>&1 | Out-Host
        $ErrorActionPreference = $oldEA
        if ($LASTEXITCODE -ne 0) { throw "Docker load failed for $tarPath." }
        Write-Ok "Loaded $tarName"
    }

    # Save rollback state
    Write-Step "Saving rollback state"
    $rollbackPath = Join-Path $BuildDir "rollback-state.json"
    $rollbackState = @{
        timestamp = (Get-Date -Format "o")
        version = $TargetVersion
        images = @()
    }

    foreach ($img in $Config.images) {
        $imageLatest = "$($img.name):latest"
        # Capture current image ID before we overwrite
        $currentId = $null
        try {
            $currentId = (docker inspect --format '{{.Id}}' $imageLatest 2>$null)
        } catch { }

        $rollbackState.images += @{
            name = $img.name
            tag = $imageLatest
            id = $currentId
            services = @($img.services)
        }
    }
    $rollbackState | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $rollbackPath -NoNewline
    Write-Ok "Rollback state saved to $rollbackPath"

    # Build env overrides
    $primaryImage = "$($Config.images[0].name):latest"
    $envOverrides = Resolve-EnvOverrides -ImageRef $primaryImage -ContainerName $containerName

    # Stop current services
    Write-Step "Stopping current services"
    Invoke-Compose -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $ComposeFile -ProjectName $projectName -EnvOverrides $envOverrides -Arguments @("down", "--remove-orphans")
    Write-Ok "Services stopped"

    # Start services
    Write-Step "Starting services"
    Invoke-Compose -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $ComposeFile -ProjectName $projectName -EnvOverrides $envOverrides -Arguments @("up", "-d", "--force-recreate")
    Write-Ok "Services started"

    # Verify (unless skipped)
    if (-not $SkipVerify) {
        Invoke-StageVerify
    }
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: VERIFY
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageVerify {
    Write-Step "Verifying deployment"

    $composeMode = Get-ComposeMode
    $containerName = Resolve-ContainerName
    $primaryImage = "$($Config.images[0].name):latest"
    $envOverrides = Resolve-EnvOverrides -ImageRef $primaryImage -ContainerName $containerName

    # Check compose ps
    Write-Detail "Compose service status:"
    Invoke-Compose -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $ComposeFile -ProjectName $projectName -EnvOverrides $envOverrides -Arguments @("ps")

    # Poll health endpoints
    $timeout = $Config.deploy.startup_timeout_seconds
    if (-not $timeout) { $timeout = 60 }
    if ($Config.deploy.health_checks) {
        foreach ($check in $Config.deploy.health_checks) {
            $port = $check.port_default
            if (-not $port) { $port = 8080 }
            if ($check.port_env -and (Test-Path -LiteralPath $EnvFile)) {
                $envPort = Read-DotEnv -FilePath $EnvFile -Key $check.port_env -DefaultValue ""
                if ($envPort) { $port = $envPort }
            }
            $url = $check.url -replace '\{port\}', $port
            $expectStatus = $check.expect_status
            if (-not $expectStatus) { $expectStatus = 200 }

            Write-Detail "Polling $($check.name): $url (timeout ${timeout}s)"

            $deadline = (Get-Date).AddSeconds($timeout)
            $healthy = $false
            while ((Get-Date) -lt $deadline) {
                try {
                    $resp = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 5 -ErrorAction SilentlyContinue
                    if ($resp.StatusCode -eq $expectStatus) {
                        $healthy = $true
                        break
                    }
                } catch {
                    # Not ready yet
                }
                Start-Sleep -Seconds 2
            }

            if ($healthy) {
                Write-Ok "$($check.name) is healthy (HTTP $expectStatus)"
            } else {
                Write-Fail "$($check.name) did not become healthy within ${timeout}s"
                throw "Health check failed: $($check.name)"
            }
        }
    }

    Write-Ok "Deployment verified"
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: ROLLBACK
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageRollback {
    Write-Step "Rolling back deployment"

    $rollbackPath = Join-Path $BuildDir "rollback-state.json"
    if (-not (Test-Path -LiteralPath $rollbackPath)) {
        throw "No rollback state found at $rollbackPath. Cannot rollback."
    }

    $state = Get-Content -LiteralPath $rollbackPath -Raw | ConvertFrom-Json
    Write-Detail "Rollback state from: $($state.timestamp)"

    if (-not (Test-Path -LiteralPath $EnvFile)) {
        throw "Env file not found: $EnvFile"
    }

    $composeMode = Get-ComposeMode
    $containerName = Resolve-ContainerName

    # Re-tag previous images
    foreach ($imgState in $state.images) {
        if ($imgState.id) {
            Write-Detail "Re-tagging $($imgState.tag) -> $($imgState.id)"
            $oldEA = $ErrorActionPreference; $ErrorActionPreference = "Continue"
            docker tag $imgState.id $imgState.tag 2>&1 | Out-Host
            $ErrorActionPreference = $oldEA
        } else {
            Write-Detail "No previous image ID for $($imgState.name), skipping re-tag"
        }
    }

    $primaryImage = "$($Config.images[0].name):latest"
    $envOverrides = Resolve-EnvOverrides -ImageRef $primaryImage -ContainerName $containerName

    # Restart stack
    Write-Detail "Restarting compose stack..."
    Invoke-Compose -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $ComposeFile -ProjectName $projectName -EnvOverrides $envOverrides -Arguments @("down", "--remove-orphans")
    Invoke-Compose -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $ComposeFile -ProjectName $projectName -EnvOverrides $envOverrides -Arguments @("up", "-d", "--force-recreate")

    # Verify
    Invoke-StageVerify

    Write-Ok "Rollback complete"
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: RESET
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageReset {
    if (-not $Force) {
        Write-Host ""
        Write-Warning "DESTRUCTIVE OPERATION"
        Write-Host "This will tear down the stack, remove volumes, and wipe all data directories."
        Write-Host "Config files (.env, release.config.json) are preserved."
        Write-Host ""
        $confirmation = Read-Host "Type 'yes' to continue"
        if ($confirmation -ne "yes") {
            throw "Reset aborted."
        }
    }

    Write-Step "Resetting $projectName"

    if (-not (Test-Path -LiteralPath $EnvFile)) {
        throw "Env file not found: $EnvFile"
    }

    $composeMode = Get-ComposeMode
    $containerName = Resolve-ContainerName
    $targetVersion = Resolve-TargetVersion
    $primaryImage = "$($Config.images[0].name):latest"
    $envOverrides = Resolve-EnvOverrides -ImageRef $primaryImage -ContainerName $containerName

    # Stop services and remove volumes
    Write-Step "Stopping services and removing volumes"
    Invoke-Compose -Mode $composeMode -EnvFilePath $EnvFile -ComposeFilePath $ComposeFile -ProjectName $projectName -EnvOverrides $envOverrides -Arguments @("down", "--remove-orphans", "-v")
    Write-Ok "Services stopped, volumes removed"

    # Detect DB driver and handle accordingly
    $databaseUrl = Read-DotEnv -FilePath $EnvFile -Key "DATABASE_URL" -DefaultValue ""
    $dbDriver = Read-DotEnv -FilePath $EnvFile -Key "DB_DRIVER" -DefaultValue ""
    if (-not $dbDriver) {
        $dbDriver = Read-DotEnv -FilePath $EnvFile -Key "DATABASE_DRIVER" -DefaultValue ""
    }
    if (-not $dbDriver) {
        if ($databaseUrl -match '^postgres(ql)?://') { $dbDriver = "postgres" }
        else { $dbDriver = "sqlite" }
    }

    if ($dbDriver -eq "postgres" -and $databaseUrl) {
        Write-Step "Resetting Postgres tables"
        $resetSql = "TRUNCATE TABLE IF EXISTS podcast_tags,podcast_items,podcasts,tags,settings,migrations,job_locks RESTART IDENTITY CASCADE;"
        $containerUrl = $databaseUrl -replace "@localhost([:/])", '@host.docker.internal$1'
        $containerUrl = $containerUrl -replace "@127\.0\.0\.1([:/])", '@host.docker.internal$1'
        docker run --rm --add-host host.docker.internal:host-gateway postgres:17-alpine `
            psql $containerUrl -v ON_ERROR_STOP=1 -c $resetSql | Out-Host
    } else {
        Write-Step "Removing SQLite database files"
        if ($Config.deploy.data_dirs) {
            $configDirSpec = $Config.deploy.data_dirs | Where-Object { $_.env -eq "HOST_CONFIG_DIR" } | Select-Object -First 1
            $configDirDefault = if ($configDirSpec) { $configDirSpec.default } else { "./config" }
            $configDir = Resolve-ProjectPath -RootDir $ScriptRoot -PathValue (Read-DotEnv -FilePath $EnvFile -Key "HOST_CONFIG_DIR" -DefaultValue $configDirDefault)
            Ensure-Directory -PathValue $configDir
            foreach ($ext in @(".db", ".db-shm", ".db-wal")) {
                $dbFile = Join-Path $configDir "$projectName$ext"
                if (Test-Path -LiteralPath $dbFile) {
                    Remove-Item -LiteralPath $dbFile -Force
                    Write-Detail "Removed $dbFile"
                }
            }
        }
    }

    # Clear data directories
    if ($Config.deploy.data_dirs) {
        foreach ($dataDir in $Config.deploy.data_dirs) {
            $dirPath = Resolve-ProjectPath -RootDir $ScriptRoot -PathValue (Read-DotEnv -FilePath $EnvFile -Key $dataDir.env -DefaultValue $dataDir.default)
            Write-Step "Clearing $($dataDir.env): $dirPath"
            Clear-DirectoryContents -PathValue $dirPath -Label $dataDir.env
        }
    }

    # Re-deploy
    Write-Step "Re-deploying from scratch"
    Invoke-StageDeploy -TargetVersion $targetVersion
    Write-Ok "Reset complete"
}

# ═══════════════════════════════════════════════════════════════════════════════
# STAGE: SHIP (full pipeline)
# ═══════════════════════════════════════════════════════════════════════════════

function Invoke-StageShip {
    if (-not $Bump -and -not $Version) {
        throw "Ship requires -Bump (major|minor|patch) or -Version X.Y.Z."
    }

    $pipelineStart = Get-Date
    $targetVersion = Resolve-TargetVersion

    Write-Host ""
    Write-Host "============================================" -ForegroundColor Cyan
    Write-Host " Ship pipeline: v$targetVersion" -ForegroundColor Cyan
    Write-Host "============================================" -ForegroundColor Cyan

    # 1. Test (before anything is committed)
    if (-not $SkipTests) {
        Invoke-StageTest
    } else {
        Write-Skip "Tests (--SkipTests)"
    }

    # 2. Bump version in files (before build so Docker gets the right version)
    Write-Step "Bumping version to $targetVersion"
    $currentVersion = Read-VersionFile -FilePath $versionFilePath
    Write-VersionFile -FilePath $versionFilePath -OldVersion $currentVersion -NewVersion $targetVersion
    if ($Config.version.sync) {
        foreach ($syncFile in $Config.version.sync) {
            $syncPath = Join-Path $ScriptRoot $syncFile
            if (Test-Path -LiteralPath $syncPath) {
                Write-VersionFile -FilePath $syncPath -OldVersion $currentVersion -NewVersion $targetVersion
            }
        }
    }

    # 3. Build (images build before git push)
    Invoke-StageBuild -TargetVersion $targetVersion

    # 4. Commit + tag + push
    Write-Step "Committing and tagging"
    $tagName = "v$targetVersion"

    Push-Location $ScriptRoot
    try {
        $commitMsg = "release: v$targetVersion"
        $addArgs = @("add", "-A", "--", ".")
        if ($Config.git -and $Config.git.exclude_paths) {
            foreach ($excl in $Config.git.exclude_paths) {
                $addArgs += ":(exclude)$excl"
            }
        }

        & git @addArgs
        if ($Config.git -and $Config.git.exclude_paths) {
            foreach ($excl in $Config.git.exclude_paths) {
                git rm --cached -r --ignore-unmatch $excl 2>$null | Out-Null
            }
        }

        git commit -m $commitMsg
        if ($LASTEXITCODE -ne 0) {
            & git @addArgs
            git commit -m $commitMsg
            if ($LASTEXITCODE -ne 0) { throw "Git commit failed." }
        }
        Write-Ok "Committed: $commitMsg"

        git tag $tagName
        if ($LASTEXITCODE -ne 0) { throw "Git tag '$tagName' failed." }
        Write-Ok "Tagged: $tagName"

        if (-not $NoPush) {
            git push $Remote main --tags
            if ($LASTEXITCODE -ne 0) { throw "Git push failed." }
            Write-Ok "Pushed to $Remote"
        } else {
            Write-Skip "Git push (--NoPush)"
        }
    } finally { Pop-Location }

    # 5. Deploy (only if env file is available)
    if (Test-Path -LiteralPath $EnvFile) {
        Invoke-StageDeploy -TargetVersion $targetVersion
    } else {
        Write-Skip "Deploy (env file not found: $EnvFile)"
    }

    $totalElapsed = Get-ElapsedString -Start $pipelineStart

    Write-Host ""
    Write-Host "============================================" -ForegroundColor Green
    Write-Host " Shipped v$targetVersion ($totalElapsed)" -ForegroundColor Green
    Write-Host "============================================" -ForegroundColor Green
    Write-Host ""
}

# ═══════════════════════════════════════════════════════════════════════════════
# DISPATCH
# ═══════════════════════════════════════════════════════════════════════════════

$pipelineStart = Get-Date

switch ($Stage) {
    "test" {
        Invoke-StageTest
    }
    "build" {
        $tv = Resolve-TargetVersion
        Invoke-StageBuild -TargetVersion $tv
    }
    "publish" {
        if (-not $Bump -and -not $Version) {
            throw "Publish requires -Bump (major|minor|patch) or -Version X.Y.Z."
        }
        $tv = Resolve-TargetVersion
        Invoke-StagePublish -TargetVersion $tv
    }
    "deploy" {
        $tv = Resolve-TargetVersion
        Invoke-StageDeploy -TargetVersion $tv
    }
    "verify" {
        Invoke-StageVerify
    }
    "rollback" {
        Invoke-StageRollback
    }
    "reset" {
        Invoke-StageReset
    }
    "ship" {
        Invoke-StageShip
    }
}

$totalElapsed = Get-ElapsedString -Start $pipelineStart
if ($Stage -ne "ship") {
    Write-Host ""
    Write-Host "Stage '$Stage' complete ($totalElapsed)" -ForegroundColor Green
    Write-Host ""
}
