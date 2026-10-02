#Requires -Version 7.0
param([Parameter(Mandatory)][string]$Image, [ValidateSet('app','cpu','cu128')][string]$Kind)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'Packaging.psm1') -Force -DisableNameChecking
$check = @'
import importlib.metadata as m, json, pathlib, re, sys
kind = sys.argv[1]
manifest = json.loads(pathlib.Path('/opt/briefcast/runtime.json').read_text())
packages = {d.metadata['Name'].lower().replace('_','-').replace('.','-'): d.version for d in m.distributions()}
assert manifest['python'] == '3.12.13'
assert manifest['architecture'] == 'x86_64'
lock = pathlib.Path('/opt/briefcast/requirements.lock' if kind == 'app' else '/opt/briefcast/requirements-' + kind + '.lock')
for line in lock.read_text().splitlines():
    match = re.match(r'^([a-zA-Z0-9_.-]+)==([^\s\\]+)', line)
    if match:
        assert m.version(match[1]) == match[2], match[1]
if kind == 'app':
    assert packages['feedparser'] == '6.0.12' and packages['mutagen'] == '1.48.1'
    assert not any(n in packages for n in ['whisperx','torch','torchaudio','ctranslate2'])
    assert not any(n.startswith('nvidia-') for n in packages)
    assert not pathlib.Path('/api/scripts/whisperx_transcribe.py').exists()
    import feedparser
    from mutagen.id3 import ID3
    assert feedparser.parse('<rss><channel><title>fixture</title></channel></rss>').feed.title == 'fixture'
    assert len(ID3()) == 0
else:
    assert set(pathlib.Path('/opt/briefcast/system-packages.lock').read_text().splitlines()) == set(manifest['system_packages'])
    assert packages['whisperx'] == '3.8.6' and packages['ctranslate2'] == '4.8.1'
    assert packages['torch'] == '2.8.0+' + kind and packages['torchaudio'] == '2.8.0+' + kind
    assert not pathlib.Path('/api/app').exists()
    import torch, torchaudio, ctranslate2, torchcodec
    assert torch.__version__ == packages['torch']
    assert manifest['pipeline_sha256']
    if kind == 'cpu': assert not any(n.startswith('nvidia-') for n in packages)
print('Image contents verified:', kind)
'@
Invoke-Docker @('run','--rm','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp:size=536870912,mode=1777','--entrypoint','python',$Image,'-c',$check,$Kind)
