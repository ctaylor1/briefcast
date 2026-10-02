#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Position = 0)][ValidateSet('test','build','publish','deploy','verify','rollback','reset','ship','')][string]$Stage = '',
    [ValidateSet('app','worker')][string]$Target = 'app',
    [ValidateSet('disabled','cpu','cuda','')][string]$Runtime = '',
    [ValidateSet('major','minor','patch','')][string]$Bump = '',
    [string]$Version = '', [string]$EnvFile = '.env', [string]$ComposeFile = 'docker-compose.yml',
    [string]$BuildDir = 'builds', [string]$Remote = 'origin',
    [switch]$SkipTests, [switch]$SkipSmoke, [switch]$SkipVerify,
    [switch]$NoCache, [switch]$NoPush, [switch]$Force
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'Packaging.psm1') -Force -DisableNameChecking
$root = Split-Path $PSScriptRoot -Parent
$config = Get-Content -LiteralPath (Join-Path $root 'release.config.json') -Raw | ConvertFrom-Json
$BuildDir = [IO.Path]::GetFullPath($BuildDir, $root)
$EnvFile = [IO.Path]::GetFullPath($EnvFile, $root)
if (-not $Stage) {
    Write-Host 'RELEASE.ps1 <test|build|publish|deploy|verify|rollback|ship> -Target app|worker -Runtime disabled|cpu|cuda [-Version X.Y.Z|-Bump patch]'
    Write-Host 'Build emits archives, checksums and immutable image manifests. Deploy never builds or stops the whole stack. PowerShell 7 and Compose >= 2.24 required.'
    exit 0
}
if ($Stage -eq 'reset') { throw 'Destructive reset is not supported by the split release pipeline. Use the reviewed recovery procedure.' }
if (-not $Runtime) {
    $Runtime = if (Test-Path -LiteralPath $EnvFile) { Get-DeploymentValue $EnvFile WHISPERX_RUNTIME disabled } else { 'disabled' }
}
if ($Target -eq 'worker' -and $Runtime -eq 'disabled') { throw 'A worker release requires -Runtime cpu or cuda.' }
$variant = if ($Target -eq 'app') { 'app' } elseif ($Runtime -eq 'cuda') { 'cu128' } else { 'cpu' }
$image = $config.images | Where-Object { $_.variant -eq $variant }
$versionConfig = $config.versions.$Target
$versionPath = Join-Path $root $versionConfig.file

function Read-Version {
    $raw = Get-Content -LiteralPath $versionPath -Raw
    if ($Target -eq 'worker') { return $raw.Trim() }
    if ($raw -match '(?m)^version\s*=\s*"([0-9]+\.[0-9]+\.[0-9]+)"') { return $Matches[1] }
    throw 'Version file is invalid.'
}
if (-not $Version) {
    $Version = Read-Version
    if ($Bump) {
        $parts = @($Version.Split('.') | ForEach-Object { [int]$_ })
        switch ($Bump) { major { $parts[0]++; $parts[1]=0; $parts[2]=0 } minor { $parts[1]++; $parts[2]=0 } patch { $parts[2]++ } }
        $Version = $parts -join '.'
    }
}
if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw 'Version must be X.Y.Z.' }
$artifactPath = Join-Path $BuildDir "$($image.name)_v$Version.json"
$rollbackPath = Join-Path $BuildDir "rollback-$Target-$Runtime.json"

function Invoke-Git {
    param([string[]]$Arguments)
    $output = & git -C $root @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Git command failed ($($Arguments[0]))." }
    return $output
}
function Get-ReleaseCompose {
    Assert-ComposeVersion
    $env:BRIEFCAST_ENV_FILE = $EnvFile
    return Get-ComposeArguments -Root $root -EnvFile $EnvFile -ComposeFile $ComposeFile -Runtime $Runtime -ProjectName $config.project
}
function Read-Artifact {
    if (-not (Test-Path -LiteralPath $artifactPath -PathType Leaf)) { throw "Missing release artifact: $artifactPath" }
    $artifact = Get-Content -LiteralPath $artifactPath -Raw | ConvertFrom-Json
    if ($artifact.version -ne $Version -or $artifact.variant -ne $variant -or $artifact.service -ne $image.service -or $artifact.image_id -notmatch '^sha256:[a-f0-9]{64}$') { throw 'Release artifact identity mismatch.' }
    return $artifact
}
function Set-ImageOverride {
    param([string]$Reference)
    if ($Reference -notmatch '^(sha256:[a-f0-9]{64}|[^\s]+@sha256:[a-f0-9]{64})$') { throw 'Deployment requires an immutable image reference.' }
    [Environment]::SetEnvironmentVariable($image.env_key, $Reference)
}
function Invoke-Test {
    foreach ($check in $config.test) {
        Push-Location (Join-Path $root $check.dir)
        try {
            Write-Host "Running $($check.name)"
            # Test commands are trusted repository configuration, never user/env input.
            Invoke-Expression $check.run
            if ($LASTEXITCODE -ne 0) { throw "Test failed: $($check.name)" }
        } finally { Pop-Location }
    }
}
function Invoke-Build {
    New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
    $revision = (Invoke-Git @('rev-parse','HEAD')).Trim()
    $dirty = [bool](Invoke-Git @('status','--porcelain'))
    $tag = "$($image.name):$Version"
    $existing = Invoke-Docker @('image','ls','--filter',"reference=$tag",'--format','{{.ID}}') -Capture
    if ($existing) { throw 'Version image tag already exists. Choose an unused version or deploy the existing artifact.' }
    $arguments = @('buildx','build','--load','--sbom=true','--provenance=mode=max','--platform','linux/amd64','-f',(Join-Path $root $image.dockerfile),'-t',$tag,
        '--build-arg',"SOURCE_REVISION=$revision")
    foreach ($arg in $image.build_args.PSObject.Properties) { $arguments += @('--build-arg', "$($arg.Name)=$($arg.Value.Replace('{version}', $Version))") }
    if ($NoCache) { $arguments += '--no-cache' }
    Invoke-Docker ($arguments + $root)
    $id = (Invoke-Docker @('image','inspect','--format','{{.Id}}',$tag) -Capture).Trim()
    if (-not $SkipSmoke) {
        $command = if ($Target -eq 'app') { '--version' } else { 'version' }
        Invoke-Docker @('run','--rm','--network','none',$id,$command)
        & (Join-Path $PSScriptRoot 'CheckImage.ps1') -Image $id -Kind $variant
    }
    $tarName = "$($image.name)_v$Version.tar"
    $tarPath = Join-Path $BuildDir $tarName
    Invoke-Docker @('image','save','--output',$tarPath,$tag)
    $checksum = (Get-FileHash -LiteralPath $tarPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $manifest = [ordered]@{schema=1; version=$Version; variant=$variant; service=$image.service; tag=$tag; image_id=$id; source_revision=$revision; source_dirty=$dirty; archive=$tarName; archive_sha256=$checksum}
    $manifest | ConvertTo-Json | Set-Content -LiteralPath $artifactPath
    "$checksum  $tarName" | Set-Content -LiteralPath "$tarPath.sha256"
    Write-Host "Built immutable artifact $artifactPath"
}
function Invoke-Publish {
    # Release only committed source. Never absorb unrelated work or staged files.
    if (Invoke-Git @('status','--porcelain')) { throw 'Publish requires a clean source tree.' }
    $prefix = if ($Target -eq 'worker') { 'worker-v' } else { 'v' }
    $tag = "$prefix$Version"
    if (Invoke-Git @('tag','--list',$tag)) { throw 'Release tag already exists.' }
    $old = Read-Version
    if ($old -ne $Version) {
        if ($Target -eq 'worker') { Set-Content -LiteralPath $versionPath -Value $Version }
        else {
            $raw = [IO.File]::ReadAllText($versionPath)
            [IO.File]::WriteAllText($versionPath, $raw.Replace("version = `"$old`"", "version = `"$Version`""))
            foreach ($file in $versionConfig.sync) {
                $path = Join-Path $root $file
                $raw = [IO.File]::ReadAllText($path)
                [IO.File]::WriteAllText($path, $raw.Replace("`"version`": `"$old`"", "`"version`": `"$Version`""))
            }
        }
        $paths = @($versionConfig.file) + @($versionConfig.sync)
        Invoke-Git (@('add','--') + $paths) | Out-Host
        Invoke-Git @('commit','-m',"release: $tag") | Out-Host
    }
    Invoke-Git @('tag','-a',$tag,'-m',"Release $tag") | Out-Host
    if (-not $NoPush) {
        Invoke-Git @('push',$Remote,'HEAD') | Out-Host
        Invoke-Git @('push',$Remote,$tag) | Out-Host
    }
}
function Assert-WorkerQuiescent {
    param([string[]]$Compose)
    if ($Target -ne 'worker') { return }
    # Require an operator drain before replacement. Snapshot/restore tooling is Stage G.
    $raw = Invoke-Docker ($Compose + @('exec','-T','whisperx','whisperx-worker','drain','--url','http://127.0.0.1:8090','--token-file','/run/secrets/whisperx-service-token')) -Capture
    $state = ($raw -join "`n") | ConvertFrom-Json
    if ($state.running -ne 0 -or $state.queued -ne 0) { throw 'Worker still has active/queued work. Settle and snapshot worker state before replacing it.' }
}
function Invoke-Deploy {
    $artifact = Read-Artifact
    $archive = Join-Path $BuildDir $artifact.archive
    if ([IO.Path]::GetFileName($artifact.archive) -ne $artifact.archive -or -not (Test-Path -LiteralPath $archive -PathType Leaf)) { throw 'Invalid or missing archive.' }
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $artifact.archive_sha256) { throw 'Archive checksum mismatch.' }
    Set-ImageOverride $artifact.image_id
    $compose = @(Get-ReleaseCompose)
    # Capture the actual running container image before loading any candidate tags.
    $container = Invoke-Docker ($compose + @('ps','-q',$image.service)) -Capture
    $oldId = $null
    if ($container) {
        if (@($container).Count -ne 1) { throw 'Expected one target container.' }
        $oldId = (Invoke-Docker @('inspect','--format','{{.Image}}',$container) -Capture).Trim()
        Assert-WorkerQuiescent $compose
    }
    Invoke-Docker @('image','load','--input',$archive)
    Invoke-Docker @('image','inspect','--format','{{.Id}}',$artifact.image_id) | Out-Host
    [ordered]@{target=$Target; runtime=$Runtime; compose=$compose; image_id=$oldId; service=$image.service; version=$Version} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $rollbackPath
    Set-ImageOverride $artifact.image_id
    Invoke-Docker ($compose + @('up','-d','--no-build','--no-deps','--force-recreate',$image.service))
    if (-not $SkipVerify) { Invoke-Verify }
}
function Invoke-Verify {
    $artifact = Read-Artifact
    Set-ImageOverride $artifact.image_id
    $compose = @(Get-ReleaseCompose)
    $container = Invoke-Docker ($compose + @('ps','-q',$image.service)) -Capture
    if (-not $container -or @($container).Count -ne 1) { throw 'Target container is not running.' }
    $id = (Invoke-Docker @('inspect','--format','{{.Image}}',$container) -Capture).Trim()
    if ($id -ne $artifact.image_id) { throw 'Running image differs from release artifact.' }
    $deadline = (Get-Date).AddSeconds(120)
    while ($true) {
        try {
            if ($Target -eq 'worker') {
                Invoke-Docker ($compose + @('exec','-T','whisperx','python','-c',"import urllib.request; urllib.request.urlopen('http://127.0.0.1:8090/health/ready',timeout=5).close()"))
            } else {
                $port = Get-DeploymentValue $EnvFile HOST_PORT 8080
                if ($port -notmatch '^\d+$') { throw 'Invalid host port.' }
                $response = Invoke-WebRequest -Uri "http://127.0.0.1:$port/" -TimeoutSec 5
                if ($response.StatusCode -ne 200) { throw 'App is not ready.' }
            }
            break
        } catch {
            if ((Get-Date) -ge $deadline) { throw 'Target readiness check timed out.' }
            Start-Sleep -Seconds 2
        }
    }
    Write-Host "Verified $Target $id"
}
function Invoke-Rollback {
    $state = Get-Content -LiteralPath $rollbackPath -Raw | ConvertFrom-Json
    $compose = @(Get-ReleaseCompose)
    if ($state.target -ne $Target -or $state.runtime -ne $Runtime -or $state.service -ne $image.service -or ($state.compose -join "`n") -ne ($compose -join "`n")) { throw 'Rollback deployment selection differs from saved state.' }
    if (-not $state.image_id) { throw 'No previous image was recorded.' }
    Assert-WorkerQuiescent $compose
    Invoke-Docker @('image','inspect','--format','{{.Id}}',$state.image_id) | Out-Host
    Set-ImageOverride $state.image_id
    Invoke-Docker ($compose + @('up','-d','--no-build','--no-deps','--force-recreate',$image.service))
    Write-Host 'Previous immutable image restored. Verify capabilities and state compatibility before resuming the worker.'
}

Push-Location $root
try {
    switch ($Stage) {
        test { Invoke-Test }
        build { Invoke-Build }
        publish { Invoke-Publish }
        deploy { Invoke-Deploy }
        verify { Invoke-Verify }
        rollback { Invoke-Rollback }
        ship {
            if (-not $SkipTests) { Invoke-Test }
            Invoke-Build
            Invoke-Publish
            Invoke-Deploy
        }
    }
} finally { Pop-Location }
