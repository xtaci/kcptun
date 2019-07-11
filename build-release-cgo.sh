#!/bin/bash

BUILD_DIR=$(dirname "$0")/build
mkdir -p $BUILD_DIR
cd $BUILD_DIR

sum="sha1sum"

if [ "$GO111MODULE" != "on" ]; then
	echo "GO111MODULE is off"
else
	echo "GO111MODULE is on"
fi 

echo "Prerequisites for cross-compiling were written in build-release.sh"

# required library for cross-compiling
# sudo apt-get install -y automake autogen build-essential ca-certificates   gcc-5-arm-linux-gnueabi g++-5-arm-linux-gnueabi libc6-dev-armel-cross   gcc-5-arm-linux-gnueabihf g++-5-arm-linux-gnueabihf libc6-dev-armhf-cross gcc-5-aarch64-linux-gnu g++-5-aarch64-linux-gnu libc6-dev-arm64-cross  gcc-5-mips-linux-gnu g++-5-mips-linux-gnu libc6-dev-mips-cross gcc-5-mipsel-linux-gnu g++-5-mipsel-linux-gnu libc6-dev-mipsel-cross  gcc-5-mips64-linux-gnuabi64 g++-5-mips64-linux-gnuabi64 libc6-dev-mips64-cross  gcc-5-mips64el-linux-gnuabi64 g++-5-mips64el-linux-gnuabi64 libc6-dev-mips64el-cross  gcc-5-multilib g++-5-multilib gcc-mingw-w64 g++-mingw-w64 clang llvm-dev   libtool libxml2-dev uuid-dev libssl-dev swig openjdk-8-jdk pkg-config patch  make xz-utils cpio wget zip unzip p7zip git mercurial bzr texinfo help2man --no-install-recommends

# if error message:
#     /usr/include/linux/errno.h:1:23: fatal error: asm/errno.h: No such file or directory
# try:
#    ln -s /usr/include/asm-generic /usr/include/asm

if ! hash sha1sum 2>/dev/null; then
	if ! hash shasum 2>/dev/null; then
		echo "I can't see 'sha1sum' or 'shasum'"
		echo "Please install one of them!"
		exit
	fi
	sum="shasum"
fi

UPX=false
if hash upx 2>/dev/null; then
	UPX=true
fi

VERSION=`date -u +%Y%m%d`
LDFLAGS="-X main.VERSION=$VERSION -s -w"
LDFLAGS_LINUX='-X main.VERSION='$VERSION' -s -w -linkmode "external" -extldflags "-static"'
LDFLAGS_LINUX32='-X main.VERSION='$VERSION' -s -w -linkmode "external" -extldflags "-static -m32 -L/usr/lib32"'
echo "-ldflag for linux/amd64:" $LDFLAGS_LINUX
echo "-ldflag for linux/386:" $LDFLAGS_LINUX32
echo "-ldflag for other:" $LDFLAGS

echo "=== Building ==="

# 386
OSES=(linux windows)
for os in ${OSES[@]}; do
	suffix=""
	if [ "$os" == "windows" ]
	then
		suffix=".exe"
	fi

	if [ "$os" == "linux" ];then
		CC=gcc-5 CGO_ENABLED=1 GOOS=$os GOARCH=386 CGO_CFLAGS="-m32 -L/usr/lib32" CGO_CXXFLAGS="-m32 -L/usr/lib32" go build -ldflags "$LDFLAGS_LINUX32" -o client_${os}_386${suffix} github.com/xtaci/kcptun/client
		CC=gcc-5 CGO_ENABLED=1 GOOS=$os GOARCH=386 CGO_CFLAGS="-m32 -L/usr/lib32" CGO_CXXFLAGS="-m32 -L/usr/lib32" go build -ldflags "$LDFLAGS_LINUX32" -o server_${os}_386${suffix} github.com/xtaci/kcptun/server
	else 
		CGO_ENABLED=0 GOOS=$os GOARCH=386 go build -ldflags "$LDFLAGS" -o client_${os}_386${suffix} github.com/xtaci/kcptun/client
		CGO_ENABLED=0 GOOS=$os GOARCH=386 go build -ldflags "$LDFLAGS" -o server_${os}_386${suffix} github.com/xtaci/kcptun/server
	fi

	if $UPX; then upx -9 client_${os}_386${suffix} server_${os}_386${suffix};fi
	tar -zcf kcptun-${os}-386-$VERSION.tar.gz client_${os}_386${suffix} server_${os}_386${suffix}
	$sum kcptun-${os}-386-$VERSION.tar.gz
done

# AMD64 
OSES=(linux darwin windows freebsd)
for os in ${OSES[@]}; do
	suffix=""
	if [ "$os" == "windows" ]
	then
		suffix=".exe"
	fi

	if [ "$os" == "linux" ];then
		CC=gcc-5 CGO_ENABLED=1 GOOS=$os GOARCH=amd64 go build -ldflags "$LDFLAGS_LINUX" -o client_${os}_amd64${suffix} github.com/xtaci/kcptun/client
		CC=gcc-5 CGO_ENABLED=1 GOOS=$os GOARCH=amd64 go build -ldflags "$LDFLAGS_LINUX" -o server_${os}_amd64${suffix} github.com/xtaci/kcptun/server
	else 
		CGO_ENABLED=0 GOOS=$os GOARCH=amd64 go build -ldflags "$LDFLAGS" -o client_${os}_amd64${suffix} github.com/xtaci/kcptun/client
		CGO_ENABLED=0 GOOS=$os GOARCH=amd64 go build -ldflags "$LDFLAGS" -o server_${os}_amd64${suffix} github.com/xtaci/kcptun/server
	fi

	if $UPX; then upx -9 client_${os}_amd64${suffix} server_${os}_amd64${suffix};fi
	tar -zcf kcptun-${os}-amd64-$VERSION.tar.gz client_${os}_amd64${suffix} server_${os}_amd64${suffix}
	$sum kcptun-${os}-amd64-$VERSION.tar.gz
done

# ARM-5
#CC=arm-linux-gnueabi-gcc-5 GOOS=linux GOARCH=arm GOARM=5 CGO_ENABLED=1 CGO_CFLAGS="-march=armv5" CGO_CXXFLAGS="-march=armv5" go install std
CC=arm-linux-gnueabi-gcc-5 CXX=arm-linux-gnueabi-g++-5 GOOS=linux GOARCH=arm GOARM=5 CGO_ENABLED=1 CGO_CFLAGS="-march=armv5" CGO_CXXFLAGS="-march=armv5" go build -ldflags "$LDFLAGS_LINUX"  -o client_linux_arm5  github.com/xtaci/kcptun/client
CC=arm-linux-gnueabi-gcc-5 CXX=arm-linux-gnueabi-g++-5 GOOS=linux GOARCH=arm GOARM=5 CGO_ENABLED=1 CGO_CFLAGS="-march=armv5" CGO_CXXFLAGS="-march=armv5" go build -ldflags "$LDFLAGS_LINUX"  -o server_linux_arm5  github.com/xtaci/kcptun/server
if $UPX; then upx -9 client_linux_arm5 server_linux_arm5;fi
tar -zcf kcptun-linux-arm5-$VERSION.tar.gz client_linux_arm5 server_linux_arm5
$sum kcptun-linux-arm5-$VERSION.tar.gz

# ARM-6
#CC=arm-linux-gnueabi-gcc-5 GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=1 CGO_CFLAGS="-march=armv6" CGO_CXXFLAGS="-march=armv6" go install std
CC=arm-linux-gnueabi-gcc-5 CXX=arm-linux-gnueabi-g++-5 GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=1 CGO_CFLAGS="-march=armv6" CGO_CXXFLAGS="-march=armv6" go build -ldflags "$LDFLAGS_LINUX"  -o client_linux_arm6 github.com/xtaci/kcptun/client
CC=arm-linux-gnueabi-gcc-5 CXX=arm-linux-gnueabi-g++-5 GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=1 CGO_CFLAGS="-march=armv6" CGO_CXXFLAGS="-march=armv6" go build -ldflags "$LDFLAGS_LINUX"  -o server_linux_arm6 github.com/xtaci/kcptun/server
if $UPX; then upx -9 client_linux_arm6 server_linux_arm6;fi
tar -zcf kcptun-linux-arm6-$VERSION.tar.gz client_linux_arm6 server_linux_arm6
$sum kcptun-linux-arm6-$VERSION.tar.gz

# ARM-7
ARMS=(7)
#CC=arm-linux-gnueabihf-gcc-5 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 CGO_CFLAGS="-march=armv7-a" CGO_CXXFLAGS="-march=armv7-a" go install std
CC=arm-linux-gnueabihf-gcc-5 CXX=arm-linux-gnueabihf-g++-5 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 CGO_CFLAGS="-march=armv7-a -fPIC" CGO_CXXFLAGS="-march=armv7-a -fPIC" go build -ldflags "$LDFLAGS_LINUX"  -o client_linux_arm7  github.com/xtaci/kcptun/client
CC=arm-linux-gnueabihf-gcc-5 CXX=arm-linux-gnueabihf-g++-5 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 CGO_CFLAGS="-march=armv7-a -fPIC" CGO_CXXFLAGS="-march=armv7-a -fPIC" go build -ldflags "$LDFLAGS_LINUX"  -o server_linux_arm7  github.com/xtaci/kcptun/server
if $UPX; then upx -9 client_linux_arm7 server_linux_arm7;fi
tar -zcf kcptun-linux-arm7-$VERSION.tar.gz client_linux_arm7 server_linux_arm7
$sum kcptun-linux-arm7-$VERSION.tar.gz

# ARM64
CC=aarch64-linux-gnu-gcc-5 CXX=aarch64-linux-gnu-g++-5 GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -ldflags "$LDFLAGS_LINUX"  -o client_linux_arm64  github.com/xtaci/kcptun/client
CC=aarch64-linux-gnu-gcc-5 CXX=aarch64-linux-gnu-g++-5 GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -ldflags "$LDFLAGS_LINUX"  -o server_linux_arm64  github.com/xtaci/kcptun/server
if $UPX; then upx -9 client_linux_arm64 server_linux_arm64*;fi
tar -zcf kcptun-linux-arm64-$VERSION.tar.gz client_linux_arm64 server_linux_arm64
$sum kcptun-linux-arm64-$VERSION.tar.gz

#MIPS32LE
CC=mipsel-linux-gnu-gcc-5 CXX=mipsel-linux-gnu-g++-5 GOOS=linux GOARCH=mipsle CGO_ENABLED=1 GOMIPS=softfloat go build -ldflags "$LDFLAGS_LINUX"  -o client_linux_mipsle github.com/xtaci/kcptun/client
CC=mipsel-linux-gnu-gcc-5 CXX=mipsel-linux-gnu-g++-5 GOOS=linux GOARCH=mipsle CGO_ENABLED=1 GOMIPS=softfloat go build -ldflags "$LDFLAGS_LINUX"  -o server_linux_mipsle github.com/xtaci/kcptun/server

#MIPS32
CC=mips-linux-gnu-gcc-5 CXX=mips-linux-gnu-g++-5 GOOS=linux GOARCH=mips CGO_ENABLED=1 GOMIPS=softfloat go build -ldflags "$LDFLAGS_LINUX"  -o client_linux_mips github.com/xtaci/kcptun/client
CC=mips-linux-gnu-gcc-5 CXX=mips-linux-gnu-g++-5 GOOS=linux GOARCH=mips CGO_ENABLED=1 GOMIPS=softfloat go build -ldflags "$LDFLAGS_LINUX"  -o server_linux_mips github.com/xtaci/kcptun/server

if $UPX; then upx -9 client_linux_mips* server_linux_mips*;fi
tar -zcf kcptun-linux-mipsle-$VERSION.tar.gz client_linux_mipsle server_linux_mipsle
tar -zcf kcptun-linux-mips-$VERSION.tar.gz client_linux_mips server_linux_mips
$sum kcptun-linux-mipsle-$VERSION.tar.gz
$sum kcptun-linux-mips-$VERSION.tar.gz

echo "=== Building Completed ==="
