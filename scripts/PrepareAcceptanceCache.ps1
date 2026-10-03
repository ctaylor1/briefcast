#Requires -Version 7.0
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Image, [Parameter(Mandatory)][string]$CacheDir)
$ErrorActionPreference = 'Stop'
$CacheDir = [IO.Path]::GetFullPath($CacheDir)
$root = [IO.Path]::GetFullPath((Split-Path $PSScriptRoot -Parent))
if ((Test-Path -LiteralPath $CacheDir) -or $CacheDir -eq $root -or $CacheDir.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Acceptance cache must be NEW and outside the repository; never refresh production caches.'
}
New-Item -ItemType Directory -Path $CacheDir | Out-Null
$prepare = @'
from pathlib import Path
from huggingface_hub import snapshot_download
revision='a29b04bd15381511a9af671baec01072039215e3'
model=snapshot_download('Systran/faster-whisper-medium.en', revision=revision)
refs=Path('/cache/huggingface/hub/models--Systran--faster-whisper-medium.en/refs')
refs.mkdir(parents=True, exist_ok=True)
(refs/'main').write_text(revision)
import nltk, whisperx, hashlib
nltk.download('punkt_tab', download_dir='/cache/nltk', raise_on_error=True)
whisperx.load_model(model, 'cpu', compute_type='int8', language='en', vad_method='pyannote')
whisperx.load_align_model(language_code='en', device='cpu')
alignment=Path('/cache/torch/hub/checkpoints/wav2vec2_fairseq_base_ls960_asr_ls960.pth')
with alignment.open('rb') as stream:
    assert hashlib.file_digest(stream,'sha256').hexdigest()=='488fd4f16de84438ffc945334278c1b9fb9b7159a806c1080b16111a958c945d'
print('Baseline ASR, VAD, alignment and NLTK cache prepared; no gated diarization models.')
'@
& docker run --rm --read-only --tmpfs /tmp:size=536870912 --entrypoint python `
    --mount "type=bind,source=$CacheDir,target=/cache" `
    --env HF_HOME=/cache/huggingface --env TORCH_HOME=/cache/torch --env NLTK_DATA=/cache/nltk `
    --env HOME=/cache/home --env XDG_CACHE_HOME=/cache/home/.cache --env HF_HUB_DOWNLOAD_TIMEOUT=30 `
    $Image -c $prepare
if ($LASTEXITCODE -ne 0) { throw 'Model cache preparation failed; no production state was changed.' }
