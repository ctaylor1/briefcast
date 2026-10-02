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
$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'scripts/ReleasePackaging.ps1') @PSBoundParameters
