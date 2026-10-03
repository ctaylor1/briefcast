#Requires -Version 7.0
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$EnvFile = $env:ENV_FILE,
    [string]$ComposeFile = $env:COMPOSE_FILE,
    [ValidateSet('disabled', 'cpu', 'cuda', '')][string]$Runtime = '',
    [switch]$SourceBuild,
    # -d otherwise binds PowerShell's common -Debug parameter and silently
    # turns `compose up -d` into attached mode. Give Docker's flag an exact alias.
    [Alias('d')][switch]$Detached,
    [string]$ProjectName = '',
    [Parameter(Position = 0, ValueFromRemainingArguments = $true)][string[]]$Arguments
)
$ErrorActionPreference = 'Stop'
if (-not $EnvFile) { $EnvFile = '.env' }
if (-not $ComposeFile) { $ComposeFile = 'docker-compose.yml' }
Import-Module (Join-Path $PSScriptRoot 'Packaging.psm1') -Force -DisableNameChecking
$root = Split-Path $PSScriptRoot -Parent
Assert-ComposeVersion
$composeArgs = Get-ComposeArguments -Root $root -EnvFile $EnvFile -ComposeFile $ComposeFile -Runtime $Runtime -SourceBuild:$SourceBuild -ProjectName $ProjectName
$env:BRIEFCAST_ENV_FILE = [IO.Path]::GetFullPath($EnvFile, $root)
if ($Runtime -eq 'cuda' -or (-not $Runtime -and (Get-DeploymentValue $env:BRIEFCAST_ENV_FILE WHISPERX_RUNTIME disabled) -eq 'cuda')) { $env:WHISPERX_BUILD_RUNTIME = 'cu128' }
if ($Detached) { $Arguments += '-d' }
Invoke-Docker -Arguments ($composeArgs + $Arguments)
