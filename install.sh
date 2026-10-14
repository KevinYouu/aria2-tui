#!/bin/bash
# Description: install aria2-tui
# Github: https://github.com/KevinYouu/aria2-tui

repo="KevinYouu/aria2-tui"

if ! command -v curl &>/dev/null; then
    echo "curl is not installed. Please install curl first."
    exit 1
fi

version=$(curl -s https://api.github.com/repos/"$repo"/releases/latest | grep 'tag_name' | cut -d\" -f4)
if [ -z "$version" ]; then
    echo "Failed to fetch the latest version. Check your internet connection or GitHub API status."
    exit 1
fi
echo "latest version: $version"

# The release tag keeps its "v" prefix in the download URL, while GoReleaser
# names the asset after the version without it (v1.0.0 -> aria2-tui_1.0.0_*.zip).
tag="$version"
version="${version#v}"

package_name=""
install_dir="/usr/local/bin"

function systemCheck() {
    system=$(uname -s)
    arch=$(uname -m)

    case "$system" in
    Darwin)
        if [ "$arch" == "x86_64" ]; then
            package_name="darwin_amd64"
            install_dir="/usr/local/bin"
        elif [ "$arch" == "arm64" ]; then
            package_name="darwin_arm64"
            install_dir="/opt/homebrew/bin"
        else
            echo "Your system is not supported."
            exit 1
        fi
        ;;
    Linux)
        if [ "$arch" == "x86_64" ]; then
            package_name="linux_amd64"
        elif [ "$arch" == "aarch64" ]; then
            package_name="linux_arm64"
        else
            echo "Your system is not supported."
            exit 1
        fi
        ;;
    *)
        echo "Your system is not supported."
        exit 1
        ;;
    esac
}

systemCheck
url=https://github.com/$repo/releases/download/$tag/aria2-tui_"$version"_"$package_name".zip
file=aria2-tui_"$version"_"$package_name".zip

echo "Downloading $url"

if ! curl -fL -o "$file" "$url"; then
    echo "Download failed"
    exit 1
fi

echo "Extracting and installing $file to $install_dir"

if sudo unzip -o "$file" -d "$install_dir"; then
    echo "Extraction successful"
else
    echo "Extraction failed"
    exit 1
fi

if sudo chmod +x "$install_dir/aria2-tui"; then
    echo "Permissions set successfully"
else
    echo "Failed to set permissions"
    exit 1
fi

rm "$file"

echo "Installation completed successfully"

if ! command -v aria2c &>/dev/null; then
    echo "aria2c was not found in PATH. Install it before running aria2-tui:"
    echo "  macOS:  brew install aria2"
    echo "  Debian: sudo apt install aria2"
    echo "  Fedora: sudo dnf install aria2"
fi

echo "Please restart your terminal or source your config manually."
