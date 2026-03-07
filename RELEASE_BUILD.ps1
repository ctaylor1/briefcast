<#
.SYNOPSIS
    Build a versioned Briefcast release: bump version, commit, tag, build Docker image, export tar.

.DESCRIPTION
    Automates the full release pipeline:
      1. Pre-flight checks (clean git, Docker available, tests pass, frontend builds)
      2. Bump version in pyproject.toml and frontend/package.json
      3. Commit all changes with "release: vX.Y.Z" message
      4. Tag the commit with "vX.Y.Z"
      5. Build Docker image with APP_VERSION build arg
      6. Export image to builds/briefcast_vX.Y.Z.tar
      7. Print deployment instructions

    Pair with RELEASE_RUN.ps1 to deploy the produced tar.

.PARAMETER Major
    Bump the major version (1.2.2 → 2.0.0).

.PARAMETER Minor
    Bump the minor version (1.2.2 → 1.3.0).

.PARAMETER Patch
    Bump the patch version (1.2.2 → 1.2.3).

.PARAMETER SkipTests
    Skip Go tests and frontend build verification before committing. Not recommended.

.PARAMETER DryRun
    Show what would happen without making any changes.

.PARAMETER ProjectDir
    Root directory of the Briefcast project. Defaults to script location.

.EXAMPLE
    .\RELEASE_BUILD.ps1 -Minor
    # Bumps minor version (1.2.2 → 1.3.0), builds, and exports tar.

.EXAMPLE
    .\RELEASE_BUILD.ps1 -Patch
    # Bumps patch version (1.2.2 → 1.2.3).

.EXAMPLE
    .\RELEASE_BUILD.ps1 -Major
    # Bumps major version (1.2.2 → 2.0.0).

.EXAMPLE
    .\RELEASE_BUILD.ps1 -Minor -DryRun
    # Shows what would happen without changing anything.
#>
[CmdletBinding()]
param(
    [switch]$Major,
    [switch]$Minor,
    [switch]$Patch,
    [switch]$SkipTests,
    [switch]$DryRun,
    [string]$ProjectDir = $PSScriptRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ─── Resolve bump type (exactly one of -Major, -Minor, -Patch is required) ───

$flagCount = [int]$Major.IsPresent + [int]$Minor.IsPresent + [int]$Patch.IsPresent
if ($flagCount -eq 0) {
    throw "You must specify a bump type: -Major, -Minor, or -Patch"
}
if ($flagCount -gt 1) {
    throw "Specify only one of -Major, -Minor, or -Patch"
}

if ($Major)     { $BumpType = "major" }
elseif ($Minor) { $BumpType = "minor" }
else            { $BumpType = "patch" }

# ─── Helpers ──────────────────────────────────────────────────────────────────

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

function Get-VersionFromPyProject {
    param([string]$FilePath)
    $content = Get-Content -LiteralPath $FilePath -Raw
    if ($content -match '(?m)^\s*version\s*=\s*"(?<version>\d+\.\d+\.\d+)"\s*$') {
        return $Matches["version"]
    }
    throw "Could not parse version from $FilePath"
}

function Get-BumpedVersion {
    param(
        [string]$CurrentVersion,
        [string]$Type
    )
    $parts = $CurrentVersion.Split(".")
    $major = [int]$parts[0]
    $minor = [int]$parts[1]
    $patch = [int]$parts[2]

    switch ($Type) {
        "major" { $major++; $minor = 0; $patch = 0 }
        "minor" { $minor++; $patch = 0 }
        "patch" { $patch++ }
    }

    return "$major.$minor.$patch"
}

function Set-VersionInFile {
    param(
        [string]$FilePath,
        [string]$OldVersion,
        [string]$NewVersion,
        [string]$Pattern
    )

    $content = Get-Content -LiteralPath $FilePath -Raw
    $replaced = $content -replace $Pattern, "`${1}$NewVersion`${2}"

    if ($replaced -eq $content) {
        throw "Failed to update version in $FilePath - pattern did not match."
    }

    Set-Content -LiteralPath $FilePath -Value $replaced -NoNewline
    Write-Detail "$FilePath`: $OldVersion -> $NewVersion"
}

function Get-ElapsedString {
    param([datetime]$Start)
    $elapsed = (Get-Date) - $Start
    if ($elapsed.TotalMinutes -ge 1) {
        return "{0:N1} min" -f $elapsed.TotalMinutes
    }
    return "{0:N0}s" -f $elapsed.TotalSeconds
}

# ─── Resolve paths ────────────────────────────────────────────────────────────

$ProjectDir = (Resolve-Path -LiteralPath $ProjectDir).Path
$pyprojectPath = Join-Path $ProjectDir "pyproject.toml"
$packageJsonPath = Join-Path $ProjectDir "frontend/package.json"
$buildsDir = Join-Path $ProjectDir "builds"
$dockerImage = "briefcast"

foreach ($required in @($pyprojectPath, $packageJsonPath)) {
    if (-not (Test-Path -LiteralPath $required)) {
        throw "Required file not found: $required"
    }
}

# ─── Step 1: Pre-flight checks ───────────────────────────────────────────────

$buildStart = Get-Date

Write-Step "Pre-flight checks"

# Docker
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker is not installed or not in PATH."
}
try { docker info 2>&1 | Out-Null } catch {
    throw "Docker daemon is not running."
}
Write-Ok "Docker is available"

# Git
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    throw "Git is not installed or not in PATH."
}

Push-Location $ProjectDir
try {
    $gitStatus = git status --porcelain
} finally {
    Pop-Location
}

# We allow uncommitted changes - the script will commit them as part of the release.
# But warn if there are untracked files outside the expected set.
if ($gitStatus) {
    Write-Detail "Working tree has uncommitted changes (they will be included in the release commit)."
} else {
    Write-Ok "Working tree is clean"
}

# ─── Step 2: Read current version and compute next ────────────────────────────

Write-Step "Version bump ($BumpType)"

$currentVersion = Get-VersionFromPyProject -FilePath $pyprojectPath
$newVersion = Get-BumpedVersion -CurrentVersion $currentVersion -Type $BumpType
$tagName = "v$newVersion"
$tarFileName = "briefcast_v$newVersion.tar"
$tarPath = Join-Path $buildsDir $tarFileName

Write-Detail "Current: $currentVersion"
Write-Detail "Next:    $newVersion ($BumpType bump)"
Write-Detail "Tag:     $tagName"
Write-Detail "Tar:     $tarPath"

# Check if tag already exists
Push-Location $ProjectDir
try {
    $existingTag = git tag -l $tagName 2>$null
} finally {
    Pop-Location
}
if ($existingTag) {
    throw "Tag '$tagName' already exists. Bump has already been done or pick a different version."
}

if ($DryRun) {
    Write-Host ""
    Write-Host "[DRY RUN] Would bump $currentVersion -> $newVersion, commit, tag, build, and export to $tarPath" -ForegroundColor Yellow
    Write-Host "[DRY RUN] No changes made." -ForegroundColor Yellow
    return
}

# ─── Step 3: Run tests (before committing) ────────────────────────────────────

if ($SkipTests) {
    Write-Step "Tests"
    Write-Skip "Skipped (-SkipTests flag)"
} else {
    Write-Step "Running Go tests"
    Push-Location $ProjectDir
    try {
        go test ./... 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "Go tests failed. Fix tests before releasing."
        }
    } finally {
        Pop-Location
    }
    Write-Ok "Go tests passed"

    Write-Step "Running frontend build check"
    Push-Location (Join-Path $ProjectDir "frontend")
    try {
        npm run build 2>&1 | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend build failed. Fix build errors before releasing."
        }
    } finally {
        Pop-Location
    }
    Write-Ok "Frontend build passed"
}

# ─── Step 4: Bump version in source files ─────────────────────────────────────

Write-Step "Updating version files"

# pyproject.toml: version = "X.Y.Z"
Set-VersionInFile `
    -FilePath $pyprojectPath `
    -OldVersion $currentVersion `
    -NewVersion $newVersion `
    -Pattern '(version\s*=\s*")[\d]+\.[\d]+\.[\d]+(")'

# frontend/package.json: "version": "X.Y.Z"
Set-VersionInFile `
    -FilePath $packageJsonPath `
    -OldVersion $currentVersion `
    -NewVersion $newVersion `
    -Pattern '("version"\s*:\s*")[\d]+\.[\d]+\.[\d]+(")'

Write-Ok "Version files updated to $newVersion"

# ─── Step 5: Git commit and tag ───────────────────────────────────────────────

Write-Step "Committing release"

Push-Location $ProjectDir
try {
    $releaseCommitMessage = "release: v$newVersion"

    git add -A
    # Never release local Codex/Claude worktree metadata as an embedded repository.
    git rm --cached -r --ignore-unmatch .claude/worktrees 2>$null | Out-Null

    git commit -m $releaseCommitMessage
    if ($LASTEXITCODE -ne 0) {
        Write-Detail "Initial commit failed; re-staging in case pre-commit hooks modified files."
        git add -A
        git rm --cached -r --ignore-unmatch .claude/worktrees 2>$null | Out-Null
        git commit -m $releaseCommitMessage
        if ($LASTEXITCODE -ne 0) {
            throw "Git commit failed after retry. Check hook output above."
        }
    }
    Write-Ok "Committed: release: v$newVersion"

    git tag $tagName
    if ($LASTEXITCODE -ne 0) {
        throw "Git tag failed."
    }
    Write-Ok "Tagged: $tagName"
} finally {
    Pop-Location
}

# ─── Step 6: Build Docker image ───────────────────────────────────────────────

Write-Step "Building Docker image (this may take several minutes)"

$dockerStart = Get-Date

Push-Location $ProjectDir
try {
    docker build `
        --build-arg APP_VERSION=$newVersion `
        -t "${dockerImage}:latest" `
        -t "${dockerImage}:${newVersion}" `
        -f Dockerfile `
        . 2>&1 | Out-Host

    if ($LASTEXITCODE -ne 0) {
        throw "Docker build failed."
    }
} finally {
    Pop-Location
}

$dockerElapsed = Get-ElapsedString -Start $dockerStart
Write-Ok "Docker image built in $dockerElapsed"
Write-Detail "Tagged: ${dockerImage}:latest, ${dockerImage}:${newVersion}"

# ─── Step 7: Export tar ───────────────────────────────────────────────────────

Write-Step "Exporting Docker image to tar"

if (-not (Test-Path -LiteralPath $buildsDir)) {
    New-Item -ItemType Directory -Path $buildsDir -Force | Out-Null
}

docker save -o $tarPath "${dockerImage}:latest"
if ($LASTEXITCODE -ne 0) {
    throw "Docker save failed."
}

$tarSize = (Get-Item -LiteralPath $tarPath).Length
$tarSizeMB = [math]::Round($tarSize / 1MB, 1)
Write-Ok "Exported: $tarPath ($tarSizeMB MB)"

# ─── Done ─────────────────────────────────────────────────────────────────────

$totalElapsed = Get-ElapsedString -Start $buildStart

Write-Host ""
Write-Host "============================================" -ForegroundColor Green
Write-Host " Release v$newVersion built successfully" -ForegroundColor Green
Write-Host " Total time: $totalElapsed" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Green
Write-Host ""
Write-Host "Artifacts:" -ForegroundColor White
Write-Host "  Image: ${dockerImage}:latest / ${dockerImage}:${newVersion}"
Write-Host "  Tar:   $tarPath"
Write-Host ""
Write-Host "To deploy:" -ForegroundColor White
Write-Host "  .\RELEASE_RUN.ps1 -Version $newVersion"
Write-Host ""
Write-Host "To push the tag:" -ForegroundColor White
Write-Host "  git push origin main --tags"
Write-Host ""
