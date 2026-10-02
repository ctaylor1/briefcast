Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-DeploymentValue {
    param([string]$EnvFile, [string]$Name, [string]$Default = '')
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ($null -ne $value) { return $value }
    foreach ($line in [IO.File]::ReadAllLines($EnvFile)) {
        if ($line -match ('^\s*' + [regex]::Escape($Name) + '\s*=\s*(.*)$')) {
            return $Matches[1].Trim().Trim('"', "'")
        }
    }
    return $Default
}

function Get-ComposeArguments {
    param([string]$Root, [string]$EnvFile, [string]$ComposeFile = 'docker-compose.yml',
          [string]$Runtime = '', [switch]$SourceBuild, [string]$ProjectName = '')
    $envPath = [IO.Path]::GetFullPath($EnvFile, $Root)
    if (-not (Test-Path -LiteralPath $envPath -PathType Leaf)) { throw 'Primary env file is required.' }
    if (-not $Runtime) { $Runtime = Get-DeploymentValue $envPath WHISPERX_RUNTIME disabled }
    $modes = Get-Content -LiteralPath (Join-Path $Root 'docker/compose-modes.json') -Raw | ConvertFrom-Json
    if ($Runtime -notin @('disabled', 'cpu', 'cuda')) { throw 'Runtime must be disabled, cpu, or cuda.' }
    $arguments = @('compose', '--project-directory', $Root, '--env-file', $envPath)
    $advanced = Get-DeploymentValue $envPath WHISPERX_ENV_FILE '.env.whisperx'
    $advancedPath = [IO.Path]::GetFullPath($advanced, $Root)
    if (Test-Path -LiteralPath $advancedPath -PathType Leaf) { $arguments += @('--env-file', $advancedPath) }
    $arguments += @('-f', [IO.Path]::GetFullPath($ComposeFile, $Root))
    foreach ($file in $modes.$Runtime.files) { $arguments += @('-f', (Join-Path $Root $file)) }
    if ($SourceBuild) {
        $arguments += @('-f', (Join-Path $Root 'docker-compose.build.yml'))
        if ($Runtime -ne 'disabled') { $arguments += @('-f', (Join-Path $Root 'docker-compose.whisperx.build.yml')) }
    }
    if ($ProjectName) { $arguments += @('-p', $ProjectName) }
    return $arguments
}

function Assert-ComposeVersion {
    $raw = & docker compose version --short
    if ($LASTEXITCODE -ne 0 -or $raw -notmatch 'v?(\d+)\.(\d+)\.(\d+)') { throw 'Docker Compose is unavailable.' }
    if ([version]$Matches[0].TrimStart('v') -lt [version]'2.24.0') { throw 'Docker Compose >= 2.24.0 is required.' }
}

function Invoke-Docker {
    param([string[]]$Arguments, [switch]$Capture)
    if ($Capture) { $output = & docker @Arguments } else { & docker @Arguments | Out-Host }
    if ($LASTEXITCODE -ne 0) { throw "Docker command failed ($($Arguments[0]))." }
    if ($Capture) { return $output }
}

Export-ModuleMember -Function Get-DeploymentValue, Get-ComposeArguments, Assert-ComposeVersion, Invoke-Docker
