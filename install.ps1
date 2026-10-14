# PowerShell install script
# Description: Install aria2-tui
# Github: https://github.com/KevinYouu/aria2-tui

# GitHub repo
$repo = "KevinYouu/aria2-tui"

# Get the latest release version
try {
    $version = (Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest").tag_name
    Write-Host "Latest version: $version"
} catch {
    Write-Host "Failed to fetch the latest version. Check your internet connection or GitHub API status."
    exit 1
}

# Check system architecture
$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch) {
    "AMD64" { $package_name = "windows_amd64" }
    "ARM64" { $package_name = "windows_arm64" }
    default {
        Write-Host "Unsupported system architecture: $arch"
        exit 1
    }
}

# Build download URL and file name. The tag keeps its "v" prefix in the URL,
# while GoReleaser names the asset after the version without it.
$tag = $version
$version = $version -replace '^v', ''
$url = "https://github.com/$repo/releases/download/$tag/aria2-tui_${version}_${package_name}.zip"
$file = "aria2-tui_${version}_${package_name}.zip"

# Download ZIP file
Write-Host "Downloading: $url"
try {
    Invoke-WebRequest -Uri $url -OutFile $file
    Write-Host "Download successful"
} catch {
    Write-Host "Download failed"
    exit 1
}

# Extract ZIP
$destination = "$env:USERPROFILE\aria2-tui"
if (-not (Test-Path $destination)) {
    New-Item -ItemType Directory -Path $destination | Out-Null
}
try {
    Expand-Archive -Path $file -DestinationPath $destination -Force
    Write-Host "Extraction successful"
} catch {
    Write-Host "Extraction failed"
    exit 1
}

$aria2TuiPath = "$destination\aria2-tui.exe"
if (-not (Test-Path $aria2TuiPath)) {
    Write-Host "Executable not found in extracted files"
    exit 1
}

# Add to PATH (current user)
$envPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($envPath -split ";" | Where-Object { $_ -eq $destination })) {
    [Environment]::SetEnvironmentVariable("Path", "$envPath;$destination", "User")
    Write-Host "aria2-tui has been added to PATH. Restart terminal to apply changes."
} else {
    Write-Host "aria2-tui is already in PATH"
}

# Clean up
Remove-Item $file -Force

if (-not (Get-Command aria2c -ErrorAction SilentlyContinue)) {
    Write-Host "aria2c was not found in PATH. Install it before running aria2-tui:"
    Write-Host "  winget install aria2.aria2"
    Write-Host "  scoop install aria2"
}

Write-Host "Installation completed successfully"
