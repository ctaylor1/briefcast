#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('snapshot','verify','restore')][string]$Operation,
    [Parameter(Mandatory)][string]$SnapshotDir,
    [Parameter(Mandatory)][string]$Image,
    [string]$StateDir = ''
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'Packaging.psm1') -Force -DisableNameChecking
$root = [IO.Path]::GetFullPath((Split-Path $PSScriptRoot -Parent))
$SnapshotDir = [IO.Path]::GetFullPath($SnapshotDir)
if ($SnapshotDir -eq $root -or $SnapshotDir.StartsWith($root + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Recovery artifacts must be outside the repository/build context in private storage.'
}
if ($Image -notmatch '^sha256:[a-f0-9]{64}$') { throw 'Recovery requires an immutable local image ID.' }
$parent = Split-Path $SnapshotDir -Parent
if (-not (Test-Path -LiteralPath $parent -PathType Container)) { throw 'Provision a private recovery parent directory first.' }
if ($Operation -eq 'snapshot' -and (Test-Path -LiteralPath $SnapshotDir)) { throw 'Snapshot destination already exists.' }
if ($Operation -ne 'snapshot' -and -not (Test-Path -LiteralPath $SnapshotDir -PathType Container)) { throw 'Snapshot is missing.' }
$arguments = @('run','--rm','--network','none','--read-only','--user','1000:1000','--cap-drop','ALL','--security-opt','no-new-privileges:true',
    '--entrypoint','python','--mount',"type=bind,source=$PSScriptRoot/whisperx_recovery.py,target=/recovery.py,readonly",
    '--mount',"type=bind,source=$parent,target=/recovery")
$snapshotName = Split-Path $SnapshotDir -Leaf
$command = @('/recovery.py', $Operation, '--snapshot', "/recovery/$snapshotName")
if ($Operation -eq 'snapshot') {
    $StateDir = [IO.Path]::GetFullPath($StateDir)
    if (-not (Test-Path -LiteralPath $StateDir -PathType Container)) { throw 'State directory is missing.' }
    $arguments += @('--mount',"type=bind,source=$StateDir,target=/state")
    $command += @('--state','/state','--image-id',$Image)
} elseif ($Operation -eq 'restore') {
    # Restore only to a new sibling directory. Selecting it in Compose is a
    # separate reviewed operation; never overwrite the current journal.
    $StateDir = [IO.Path]::GetFullPath($StateDir)
    if (Test-Path -LiteralPath $StateDir) { throw 'Restore destination must be new.' }
    $stateParent = Split-Path $StateDir -Parent
    if (-not (Test-Path -LiteralPath $stateParent -PathType Container)) { throw 'Restore parent is missing.' }
    $arguments += @('--mount',"type=bind,source=$stateParent,target=/restore")
    $command += @('--state',('/restore/' + (Split-Path $StateDir -Leaf)))
}
Invoke-Docker ($arguments + @($Image) + $command)
