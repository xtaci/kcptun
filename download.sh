#!/usr/bin/env bash

# Detect the operating system type and architecture
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Map architecture to the corresponding string used in the release names
case "$ARCH" in
    x86_64 | amd64)
        ARCH="amd64"
        ;;
    i386 | i686)
        ARCH="386"
        ;;
    armv5*)
        ARCH="arm5"
        ;;
    armv6*)
        ARCH="arm6"
        ;;
    armv7*)
        ARCH="arm7"
        ;;
    aarch64 | arm64)
        ARCH="arm64"
        ;;
    mips)
        ARCH="mips"
        ;;
    mipsle)
        ARCH="mipsle"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Determine the operating system
case "$OS" in
    linux | darwin | freebsd | windows)
        OS="$OS"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Get the latest version number and remove the 'v' prefix
LATEST_VERSION=$(curl -s https://api.github.com/repos/xtaci/kcptun/releases/latest | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/')

# Construct the download URL
DOWNLOAD_URL="https://github.com/xtaci/kcptun/releases/download/v$LATEST_VERSION/kcptun-$OS-$ARCH-$LATEST_VERSION.tar.gz"

# Display the download URL
echo "Constructed download URL: $DOWNLOAD_URL"

# Download the file
echo "Downloading kcptun $LATEST_VERSION for $OS/$ARCH..."
curl -L -O $DOWNLOAD_URL

# Extract the filename from the URL
FILENAME=$(basename $DOWNLOAD_URL)

# Check if the download was successful
if [ $? -eq 0 ]; then
    echo "Download complete: $FILENAME"
else
    echo "Download failed. Please check if the OS/ARCH combination is supported or if the URL is correct."
fi
