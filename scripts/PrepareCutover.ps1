#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$EnvFile,
    [Parameter(Mandatory)][string]$ArtifactsDir,
    [Parameter(Mandatory)][string]$RecoveryDir,
    [Parameter(Mandatory)][string]$OutputFile,
    [ValidateSet('cpu','cuda')][string]$Runtime = 'cuda',
    [string]$AppVersion = '1.9.4', [string]$WorkerVersion = '1.0.0',
    [string]$CorpusEvidence = '',
    [string]$SourceBundle = ''
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'Packaging.psm1') -Force -DisableNameChecking
$root = Split-Path $PSScriptRoot -Parent
$OutputFile = [IO.Path]::GetFullPath($OutputFile)
if ((Test-Path -LiteralPath $OutputFile) -or $OutputFile -eq $root -or $OutputFile.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Select a new private output file outside the repository.' }
if (-not (Test-Path -LiteralPath (Split-Path $OutputFile -Parent) -PathType Container)) { throw 'Provision a private output parent first.' }
if ($IsWindows) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    # Output records contain private operational paths. Restrict the NEW file,
    # without changing permissions on any operator-supplied parent directory.
    New-Item -ItemType File -Path $OutputFile | Out-Null
    & icacls $OutputFile /inheritance:r /grant:r ($identity + ':F') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Private cutover record permissions failed.' }
}
$variant = if ($Runtime -eq 'cuda') { 'cu128' } else { 'cpu' }
function Read-VerifiedArtifact([string]$Name) {
    $artifact = Get-Content -LiteralPath (Join-Path $ArtifactsDir $Name) -Raw | ConvertFrom-Json
    if ($artifact.image_id -notmatch '^sha256:[a-f0-9]{64}$' -or [IO.Path]::GetFileName($artifact.archive) -ne $artifact.archive) { throw 'Invalid immutable release artifact.' }
    $archive = Join-Path $ArtifactsDir $artifact.archive
    if ((Get-FileHash -LiteralPath $archive).Hash.ToLowerInvariant() -ne $artifact.archive_sha256) { throw 'Release archive checksum mismatch.' }
    return $artifact
}
$app = Read-VerifiedArtifact "briefcast_v$AppVersion.json"
$worker = Read-VerifiedArtifact "briefcast-whisperx-$($variant)_v$WorkerVersion.json"
if ($app.schema -ne 1 -or $app.service -ne 'briefcast' -or $app.version -ne $AppVersion -or $worker.schema -ne 1 -or $worker.service -ne 'whisperx' -or $worker.version -ne $WorkerVersion -or $worker.variant -ne $variant) { throw 'Artifact service/version/variant does not match this cutover.' }
$recovery = Get-Content -LiteralPath (Join-Path $RecoveryDir 'manifest.json') -Raw | ConvertFrom-Json
$database = Join-Path $RecoveryDir 'database-restore.json'
$assets = Join-Path $RecoveryDir 'assets-protection.json'
$quality = $false
if ($CorpusEvidence -and (Test-Path -LiteralPath $CorpusEvidence -PathType Leaf)) {
    $evidence = Get-Content -LiteralPath $CorpusEvidence -Raw | ConvertFrom-Json
    $quality = $evidence.quality_passed -eq $true -and $evidence.recovery_passed -eq $true -and $evidence.device -eq $Runtime -and $evidence.worker_image -eq $worker.image_id -and $evidence.representative_corpus -eq $true
}
$imageVerified = $false
if ($recovery.image_archive_sha256) {
    $imageVerified = (Get-FileHash -LiteralPath (Join-Path $RecoveryDir 'monolith.tar')).Hash.ToLowerInvariant() -eq $recovery.image_archive_sha256
}
$databaseVerified = $false
if (Test-Path -LiteralPath $database) {
    $restore = Get-Content -LiteralPath $database -Raw | ConvertFrom-Json
    $databaseVerified = $restore.complete -eq $true -and $restore.archived_app_boot -eq $true -and $restore.catalog_and_settings_served -eq $true -and $restore.dump_sha256 -eq $recovery.database_sha256 -and (Get-FileHash -LiteralPath (Join-Path $RecoveryDir 'database.dump')).Hash.ToLowerInvariant() -eq $restore.dump_sha256
}
$assetsVerified = $false
if (Test-Path -LiteralPath $assets) {
    $protection = Get-Content -LiteralPath $assets -Raw | ConvertFrom-Json
    $assetsVerified = $protection.verified -eq $true -and $protection.coherent_with_database_sha256 -eq $recovery.database_sha256
}
$envPath = [IO.Path]::GetFullPath($EnvFile)
$configVerified = $false
if ($recovery.config_archive_sha256) { $configVerified = (Get-FileHash -LiteralPath (Join-Path $RecoveryDir 'config.tar')).Hash.ToLowerInvariant() -eq $recovery.config_archive_sha256 }
$bundleVerified = $false
if ($SourceBundle) {
    & git -C $root bundle verify $SourceBundle 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Candidate source bundle verification failed.' }
    $heads = & git -C $root bundle list-heads $SourceBundle
    $head = (& git -C $root rev-parse HEAD).Trim()
    $bundleVerified = @($heads | Where-Object { $_.StartsWith($head + ' ') }).Count -gt 0 -and -not (& git -C $root status --porcelain)
}
$gates = [ordered]@{archive=$imageVerified; database_restore=$databaseVerified; configuration_archive=$configVerified; candidate_source_bundle=$bundleVerified; consistent_assets=$assetsVerified; representative_quality=$quality}
$ready = @($gates.Values | Where-Object { -not $_ }).Count -eq 0
$selection = @(Get-ComposeArguments -Root $root -EnvFile $envPath -Runtime $Runtime)
$record = [ordered]@{
    schema=1; ready_for_maintenance_review=$ready; gates=$gates; runtime=$Runtime;
    source_revision=(& git -C $root rev-parse HEAD).Trim();
    app=$app; worker=$worker; baseline_image=$recovery.image_id;
    env_file=$envPath; env_sha256=(Get-FileHash -LiteralPath $envPath).Hash.ToLowerInvariant();
    compose_arguments=$selection; image_overrides=@{BRIEFCAST_IMAGE=$app.image_id; WHISPERX_IMAGE=$worker.image_id};
    recovery_directory=[IO.Path]::GetFullPath($RecoveryDir); recovery_manifest=$recovery;
    corpus_evidence=$CorpusEvidence; source_bundle=$SourceBundle;
    source_bundle_sha256=$(if ($SourceBundle) { (Get-FileHash -LiteralPath $SourceBundle).Hash.ToLowerInvariant() } else { '' });
    commands=[ordered]@{
        worker_start=@('docker') + $selection + @('up','-d','--no-build','--no-deps','whisperx');
        app_start=@('docker') + $selection + @('up','-d','--no-build','--no-deps','briefcast');
        worker_drain=@('pwsh','-File',(Join-Path $root 'RELEASE.ps1'),'drain','-Target','worker','-Runtime',$Runtime,'-EnvFile',$envPath);
        split_stop=@('docker') + $selection + @('stop','briefcast','whisperx');
        remote_reset_dry_run=@('go','run','./cmd/transcription-rollback');
        remote_reset_apply_example=@('go','run','./cmd/transcription-rollback','--apply','--services-stopped','--backup','REVIEWED_NEW_PRIVATE_BACKUP_PATH');
        legacy_start=@('docker','compose','--project-directory',$root,'--env-file','RESTORED_PRIVATE_BASELINE_ENV','-f',(Join-Path $root 'docker-compose.legacy.yml'),'-f',(Join-Path $root 'docker-compose.legacy-gpu.yml'),'up','-d','--no-build','--no-deps','briefcast')
    };
    command_requirements=@(
        'Apply image_overrides in the command environment; record inherits no permission to deploy.',
        'Use the captured database settings privately for the offline rollback CLI; both split services must already be stopped.',
        'Legacy start requires restoring BRIEFCAST_IMAGE to baseline_image and the original effective settings and mounts.'
        'Legacy Compose service env_file also references repository .env: restore its private baseline copy and advanced dotenv file before running the recorded command.'
    );
    ordered_actions=@(
        'Wait for active local work; stop the inspected monolith before any candidate app database access.',
        'Take the FINAL coherent database/config/media checkpoint and reverify all gate artifacts.',
        'Start and verify only the pinned worker; preserve Postgres and all baseline artifacts.',
        'Deploy only the pinned lean app with HTTP backend, original internal paths and baseline settings.',
        'Verify one canary transcript/export and summary; record temporary dispatch limits and restore them.',
        'Observe the representative long episode and scheduling interval; retain 24-hour operational gate.'
    );
    rollback_actions=@(
        'Drain and stop both split services; preserve current completed transcripts/summaries and candidate journal.',
        'Run cmd/transcription-rollback dry-run against this database; review inspected IDs and back up before apply.',
        'Preserve legacy media sidecars separately before the archived local script can overwrite them; reuse only proven compatible checkpoints.',
        'Use --apply --services-stopped only for the inspected unfinished remote envelopes.',
        'Restore private original configuration and exact baseline image. Keep current database/media absent a separate corruption recovery decision.'
    )
}
$record | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $OutputFile
if (-not $IsWindows) {
    & chmod 600 $OutputFile
    if ($LASTEXITCODE -ne 0) { throw 'Private cutover record permissions failed.' }
}
Write-Host "Prepared private first-cutover record; ready for maintenance review: $ready. No deployment was performed."
