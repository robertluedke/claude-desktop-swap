# Installs or updates claude-desktop-swap for the current Windows user:
#   irm https://raw.githubusercontent.com/robertluedke/claude-desktop-swap/main/install.ps1 | iex
param(
    [string]$Url
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$repo = 'robertluedke/claude-desktop-swap'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
if (-not $Url) { $Url = "https://github.com/$repo/releases/latest/download/claude-desktop-swap_windows_$arch.zip" }
$dir = Join-Path $env:LOCALAPPDATA 'Programs\claude-desktop-swap'
$exe = Join-Path $dir 'claude-desktop-swap.exe'
$alias = Join-Path $dir 'cdswap.exe'

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("cdswap-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading $Url"
    $zip = Join-Path $tmp 'cdswap.zip'
    Invoke-WebRequest -Uri $Url -OutFile $zip -UseBasicParsing
    Expand-Archive -Path $zip -DestinationPath $tmp -Force

    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    # The alias is a hard link to the exe, so it has to go before the exe can
    # be replaced; a running instance keeps its own handle and is unaffected.
    Remove-Item $alias, $exe -Force -ErrorAction SilentlyContinue
    Copy-Item (Join-Path $tmp 'claude-desktop-swap.exe') $exe
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# A hard link needs neither admin rights nor Developer Mode, works from every
# shell, and costs no extra disk space; fall back to a copy if the volume refuses.
try { New-Item -ItemType HardLink -Path $alias -Target $exe | Out-Null }
catch { Copy-Item $exe $alias }

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | Where-Object { $_ })
if ($entries -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
    Write-Host "Added $dir to your user PATH."
}
if (($env:Path -split ';') -notcontains $dir) { $env:Path += ";$dir" }

Write-Host ''
& $exe --version
Write-Host 'Installed. Run "claude-desktop-swap" or "cdswap" from any terminal (open a new one if this one does not find it).'
