#!/usr/bin/env bash
# kcptun build-release.sh - Final Corrected Version

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
# Target platforms for cross-compilation. Format: OS/ARCH/GOARM/GOMIPS
# GOARM (5, 6, 7) and GOMIPS (softfloat) are included for specific architectures.
TARGETS=(
    "darwin/amd64//" 
    "darwin/arm64//"
    
    "freebsd/amd64//"
    
    "linux/386//" 
    "linux/amd64//" 
    "linux/arm/5/"          # ARMv5
    "linux/arm/6/"          # ARMv6
    "linux/arm/7/"          # ARMv7
    "linux/arm64//" 
    "linux/loong64//"
    "linux/mips//softfloat" 
    "linux/mipsle//softfloat" 
    
    "windows/386//" 
    "windows/amd64//" 
    "windows/arm64//"
)

# Full Go package import paths
CLIENT_SRC="github.com/xtaci/kcptun/client"
SERVER_SRC="github.com/xtaci/kcptun/server"

# Build output directory
BUILD_DIR="$(pwd)/build"

# Version based on UTC date and linker flags
VERSION=$(date -u +%Y%m%d)
LDFLAGS="-X main.VERSION=${VERSION} -s -w"

# --- Tool Check ---
# Determine the SHA checksum utility (sha1sum or shasum)
if command -v sha1sum &> /dev/null; then
    SUM_TOOL="sha1sum"
elif command -v shasum &> /dev/null; then
    SUM_TOOL="shasum"
else
    echo "Error: Neither 'sha1sum' nor 'shasum' tool found."
    exit 1
fi

# Check for UPX compressor availability
if command -v upx &> /dev/null; then
    USE_UPX=true
    echo "Info: UPX found. Binaries will be compressed."
else
    USE_UPX=false
    echo "Info: UPX not found. Skipping compression step."
fi

# Enable Go module mode
export GO111MODULE=on

# --- Core Functions ---

# Determines the unique file suffix (e.g., 'amd64', 'arm5').
get_suffix() {
    local arch=$1
    local goarm=$2
    
    local suffix="${arch}"
    if [ ! -z "${goarm}" ]; then
        suffix="${arch}${goarm}" # Concatenate arch and GOARM (e.g., arm5)
    fi
    echo "${suffix}"
}

# Compiles binaries for the given target, applies UPX.
build_target() {
    local os=$1
    local arch=$2
    local goarm=$3
    local gomips=$4
    
    # Add .exe extension for Windows targets
    local ext=""
    if [ "${os}" == "windows" ]; then
        ext=".exe"
    fi

    local suffix=$(get_suffix "${arch}" "${goarm}")

    local client_out="${BUILD_DIR}/client_${os}_${suffix}${ext}"
    local server_out="${BUILD_DIR}/server_${os}_${suffix}${ext}"

    echo "--- Building ${os}/${suffix} ---"

    # Set Go cross-compilation environment variables
    export CGO_ENABLED=0
    export GOOS=${os}
    export GOARCH=${arch}
    export GOARM=${goarm}
    export GOMIPS=${gomips}

    # Execute Go build
    go build -mod=vendor -ldflags "${LDFLAGS}" -o "${client_out}" "${CLIENT_SRC}" || { echo "Error: Client compilation failed for ${os}/${suffix}"; return 1; }
    go build -mod=vendor -ldflags "${LDFLAGS}" -o "${server_out}" "${SERVER_SRC}" || { echo "Error: Server compilation failed for ${os}/${suffix}"; return 1; }

    # UPX compression
    if $USE_UPX; then
        echo "Compressing binaries using UPX..."
        upx "${client_out}" "${server_out}" || { echo "Warning: UPX compression failed."; }
    fi
}

# Packages the compiled binaries into a tar.gz archive.
# The archive directly contains the two binaries (client/server).
package_target() {
    local os=$1
    local arch=$2
    local goarm=$3
    local gomips=$4

    # Determine file extension and suffix
    local ext=""
    if [ "${os}" == "windows" ]; then
        ext=".exe"
    fi
    local suffix=$(get_suffix "${arch}" "${goarm}") 

    # Full filenames in the BUILD_DIR
    local client_bin="client_${os}_${suffix}${ext}"
    local server_bin="server_${os}_${suffix}${ext}"
    
    # Archive name format: kcptun-os-suffix-VERSION.tar.gz
    local package_name="kcptun-${os}-${suffix}-${VERSION}"
    local archive_file="${package_name}.tar.gz"

    echo "--- Packaging ${package_name} ---"

    # Create the tarball directly from the binaries (no intermediate directory inside the archive)
    (
        cd "${BUILD_DIR}" && \
        tar -czf "${archive_file}" "${client_bin}" "${server_bin}"
    )

    if [ $? -ne 0 ]; then
        echo "Error: Failed to create package ${archive_file}"
        return 1
    fi
}

# --- Main Execution ---

# 1. Initialize the build directory
mkdir -p "${BUILD_DIR}" || { echo "Error: Could not create build directory ${BUILD_DIR}"; exit 1; }

# 2. Loop through targets for building and packaging
for target in "${TARGETS[@]}"; do
    # Split the target string: OS/ARCH/GOARM/GOMIPS
    IFS='/' read -r OS ARCH GOARM GOMIPS <<< "$target"

    build_target "$OS" "$ARCH" "$GOARM" "$GOMIPS" || exit 1
    package_target "$OS" "$ARCH" "$GOARM" "$GOMIPS" || exit 1
done

# 3. Clean up intermediate binaries (client_* and server_* files)
echo "--- Cleaning intermediate binaries ---"
find "${BUILD_DIR}" -type f -regex "${BUILD_DIR}/\(client\|server\)_.*" -delete

# 4. Generate SHA1 checksums file
echo "--- Generating SHA1 Checksums ---"
(cd "${BUILD_DIR}" && $SUM_TOOL *.tar.gz > SHA1SUMS)

# 5. Output checksums to console 
echo "--- SHA1SUMS Output ---"
cat "${BUILD_DIR}/SHA1SUMS"
echo "---"

echo "--- Build Complete ---"
echo "All release packages are located in ${BUILD_DIR}/"