Param(
    [switch]$SkipIntegration,
    [switch]$SkipWhisperX,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"

Write-Host "Running Go regression tests..."
go test ./...

if (-not $SkipIntegration) {
    if ($env:BRIEFCAST_INTEGRATION -ne "1") {
        Write-Host "Skipping integration test (set BRIEFCAST_INTEGRATION=1 to enable)."
    } else {
        Write-Host "Running integration test..."
        go test ./service -run TestIntegrationFeedDownloadWhisperX
    }
}

if (-not $SkipFrontend) {
    Write-Host "Running frontend regression (build/typecheck)..."
    npm --prefix frontend run test
}

if (-not $SkipWhisperX) {
    if ($env:BRIEFCAST_WHISPERX_REAL -ne "1") {
        Write-Host "Skipping WhisperX regression (set BRIEFCAST_WHISPERX_REAL=1 and WHISPERX_TEST_AUDIO=path to enable)."
    } else {
        Write-Host "Running WhisperX regression..."
        go test ./service -run TestWhisperXRealTranscription
    }
}
