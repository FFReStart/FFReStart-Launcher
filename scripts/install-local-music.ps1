param(
    [string]$SourceRepository = 'C:\Users\mcdra\Documents\GitHub\FFReStart-Launcher',
    [string]$SourceRef = '0-zach/launcher-auth-ui-integration'
)

$ErrorActionPreference = 'Stop'
if (-not $env:LOCALAPPDATA) { throw 'LOCALAPPDATA is unavailable.' }

$audioDirectory = Join-Path $env:LOCALAPPDATA 'FFReStart\launcher\audio'
$destination = Join-Path $audioDirectory 'launcher-main-theme.mp3'
$temporary = "$destination.tmp"
[System.IO.Directory]::CreateDirectory($audioDirectory) | Out-Null

# Use git show without checking out or modifying the owner's source branch.
$start = [System.Diagnostics.ProcessStartInfo]::new()
$start.FileName = 'git'
$start.UseShellExecute = $false
$start.RedirectStandardOutput = $true
$start.RedirectStandardError = $true
$start.Arguments = "-C `"$SourceRepository`" show `"${SourceRef}:GameLauncher/audio/launcher-main-theme.mp3`""
$process = [System.Diagnostics.Process]::Start($start)
$output = [System.IO.File]::Open($temporary, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write)
try { $process.StandardOutput.BaseStream.CopyTo($output) } finally { $output.Dispose() }
$errorText = $process.StandardError.ReadToEnd()
$process.WaitForExit()
if ($process.ExitCode -ne 0) { Remove-Item -LiteralPath $temporary -ErrorAction SilentlyContinue; throw "git show failed: $errorText" }
Move-Item -LiteralPath $temporary -Destination $destination -Force
Write-Host "Installed private launcher music at $destination"
