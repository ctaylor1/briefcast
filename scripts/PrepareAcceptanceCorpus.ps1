#Requires -Version 7.0
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Image, [Parameter(Mandatory)][string]$CorpusDir)
$ErrorActionPreference = 'Stop'
$CorpusDir = [IO.Path]::GetFullPath($CorpusDir)
$root = Split-Path $PSScriptRoot -Parent
if ((Test-Path -LiteralPath $CorpusDir) -or $CorpusDir -eq $root -or $CorpusDir.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Select a new corpus directory outside the repository.' }
New-Item -ItemType Directory -Path $CorpusDir | Out-Null
if ($IsWindows) {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    & icacls $CorpusDir /inheritance:r /grant:r ($identity + ':(OI)(CI)F') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Private corpus permissions failed.' }
}
$source = 'https://static.ubuntupodcast.org/ubuntupodcast/s14/e30/ubuntupodcast_s14e30.mp3'
$original = Join-Path $CorpusDir 'original.mp3'
Invoke-WebRequest -Uri $source -OutFile $original -TimeoutSec 90
$sourceHash = (Get-FileHash -LiteralPath $original).Hash.ToLowerInvariant()
if ($sourceHash -ne '063cfe433f1e636b3785a06fcf709a1c4a2416658808fa54d2a598900f1281f4') { throw 'Public corpus source changed; review it before decoding.' }
$imageId = (& docker image inspect --format '{{.Id}}' $Image).Trim()
if ($LASTEXITCODE -ne 0 -or $imageId -notmatch '^sha256:[a-f0-9]{64}$') { throw 'Resolve the pinned worker image first.' }
$taskUid = if ($IsWindows) { '1000' } else { (& id -u).Trim() }
$taskGid = if ($IsWindows) { '1000' } else { (& id -g).Trim() }
$decode = @('run','--rm','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true',
    '--user',"$($taskUid):$taskGid",'--tmpfs','/tmp:size=134217728','--entrypoint','ffmpeg',
    '--mount',"type=bind,source=$CorpusDir,target=/corpus",$imageId,'-hide_banner','-loglevel','error')
foreach ($audioArgs in @(
    ,@('-ss','60','-i','/corpus/original.mp3','-t','300','-ar','16000','-ac','1','/corpus/ubuntu-300s.wav'),
    ,@('-ss','60','-i','/corpus/original.mp3','-t','40','-c:a','flac','/corpus/ubuntu-excerpt.flac'),
    ,@('-f','lavfi','-i','anullsrc=channel_layout=mono:sample_rate=16000','-t','10','/corpus/silence.wav')
)) {
    & docker @decode @audioArgs
    if ($LASTEXITCODE -ne 0) { throw 'Corpus decoding failed.' }
}
$files = @{}
foreach ($name in @('ubuntu-300s.wav','ubuntu-excerpt.flac','silence.wav')) { $files[$name] = (Get-FileHash -LiteralPath (Join-Path $CorpusDir $name)).Hash.ToLowerInvariant() }
[ordered]@{
    schema=1; source=$source; source_sha256=$sourceHash; image_id=$imageId; files=$files;
    attribution='Ubuntu Podcast S14E30, Ubuntu UK Local Community Team and presenters';
    license='CC BY-SA 3.0'; license_evidence='https://ubuntupodcast.org/about/';
    transformations='300-second mono WAV and 40-second FLAC excerpts starting at 60 seconds; generated 10-second silence';
    distribution='Keep attribution and share-alike terms with redistributed audio/transcript derivatives. Corpus remains outside Git.'
} | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $CorpusDir 'corpus.json')
Write-Host 'Prepared pinned public podcast excerpts and silence with attribution; no production media was used.'
