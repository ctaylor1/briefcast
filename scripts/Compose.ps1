#Requires -Version 7.0
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$EnvFile = '.env',
    [string]$ComposeFile = 'docker-compose.yml',
    [ValidateSet('disabled', 'cpu', 'cuda', '')][string]$Runtime = '',
    [switch]$SourceBuild,
    [string]$ProjectName = '',
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)][string[]]$Arguments
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'Packaging.psm1') -Force -DisableNameChecking
$root = Split-Path $PSScriptRoot -Parent
Assert-ComposeVersion
$composeArgs = Get-ComposeArguments -Root $root -EnvFile $EnvFile -ComposeFile $ComposeFile -Runtime $Runtime -SourceBuild:$SourceBuild -ProjectName $ProjectName
$env:BRIEFCAST_ENV_FILE = [IO.Path]::GetFullPath($EnvFile, $root)
if ($Runtime -eq 'cuda' -or (-not $Runtime -and (Get-DeploymentValue $env:BRIEFCAST_ENV_FILE WHISPERX_RUNTIME disabled) -eq 'cuda')) { $env:WHISPERX_BUILD_RUNTIME = 'cu128' }
Invoke-Docker -Arguments ($composeArgs + $Arguments)
