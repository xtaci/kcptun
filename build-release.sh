#!/usr/bin/env bash
 
BUILD_DIR=$(dirname "$0")/build
mkdir -p $BUILD_DIR
cd $BUILD_DIR

sum="sha1sum"
COMPRESS="gzip"
if hash pigz 2>/dev/null; then
    COMPRESS="pigz"
fi

export GO111MODULE=on
echo "Setting GO111MODULE to" $GO111MODULE

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
GCFLAGS=""

# LOONG64
OSES=(linux)
for os in ${OSES[@]}; do
    env CGO_ENABLED=0 GOOS=$os GOARCH=loong64 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_${os}_loong64${suffix} github.com/xtaci/kcptun/client
    env CGO_ENABLED=0 GOOS=$os GOARCH=loong64 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_${os}_loong64${suffix} github.com/xtaci/kcptun/server
    if $UPX; then upx -9 client_${os}_loong64${suffix} server_${os}_loong64${suffix};fi
    tar -cf kcptun-${os}-loong64-$VERSION.tar client_${os}_loong64${suffix} server_${os}_loong64${suffix}
    ${COMPRESS} -f kcptun-${os}-loong64-$VERSION.tar
    $sum kcptun-${os}-loong64-$VERSION.tar.gz
done

# AMD64 
OSES=(linux darwin windows freebsd)
for os in ${OSES[@]}; do
    suffix=""
    if [ "$os" == "windows" ]
    then
        suffix=".exe"
    fi
    env CGO_ENABLED=0 GOOS=$os GOARCH=amd64 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_${os}_amd64${suffix} github.com/xtaci/kcptun/client
    env CGO_ENABLED=0 GOOS=$os GOARCH=amd64 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_${os}_amd64${suffix} github.com/xtaci/kcptun/server
    if $UPX; then upx -9 client_${os}_amd64${suffix} server_${os}_amd64${suffix};fi
    tar -cf kcptun-${os}-amd64-$VERSION.tar client_${os}_amd64${suffix} server_${os}_amd64${suffix}
    ${COMPRESS} -f kcptun-${os}-amd64-$VERSION.tar
    $sum kcptun-${os}-amd64-$VERSION.tar.gz
done

# 386
OSES=(linux windows)
for os in ${OSES[@]}; do
    suffix=""
    if [ "$os" == "windows" ]
    then
        suffix=".exe"
    fi
    env CGO_ENABLED=0 GOOS=$os GOARCH=386 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_${os}_386${suffix} github.com/xtaci/kcptun/client
    env CGO_ENABLED=0 GOOS=$os GOARCH=386 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_${os}_386${suffix} github.com/xtaci/kcptun/server
    if $UPX; then upx -9 client_${os}_386${suffix} server_${os}_386${suffix};fi
    tar -cf kcptun-${os}-386-$VERSION.tar client_${os}_386${suffix} server_${os}_386${suffix}
    ${COMPRESS} -f kcptun-${os}-386-$VERSION.tar
    $sum kcptun-${os}-386-$VERSION.tar.gz
done

# ARM
ARMS=(5 6 7)
for v in ${ARMS[@]}; do
    env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=$v go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_linux_arm$v  github.com/xtaci/kcptun/client
    env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=$v go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_linux_arm$v  github.com/xtaci/kcptun/server

    if $UPX; then upx -9 client_linux_arm$v server_linux_arm$v;fi
    tar -cf kcptun-linux-arm$v-$VERSION.tar client_linux_arm$v server_linux_arm$v
    ${COMPRESS} -f kcptun-linux-arm$v-$VERSION.tar
    $sum kcptun-linux-arm$v-$VERSION.tar.gz
done

# ARM64
OSES=(linux darwin windows)
for os in ${OSES[@]}; do
    suffix=""
    if [ "$os" == "windows" ]
    then
        suffix=".exe"
    fi
    env CGO_ENABLED=0 GOOS=$os GOARCH=arm64 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_${os}_arm64${suffix} github.com/xtaci/kcptun/client
    env CGO_ENABLED=0 GOOS=$os GOARCH=arm64 go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_${os}_arm64${suffix} github.com/xtaci/kcptun/server
    if $UPX; then upx -9 client_${os}_arm64${suffix} server_${os}_arm64${suffix};fi
    tar -cf kcptun-${os}-arm64-$VERSION.tar client_${os}_arm64${suffix} server_${os}_arm64${suffix}
    ${COMPRESS} -f kcptun-${os}-arm64-$VERSION.tar
    $sum kcptun-${os}-arm64-$VERSION.tar.gz
done

#MIPS32LE
env CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_linux_mipsle github.com/xtaci/kcptun/client
env CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_linux_mipsle github.com/xtaci/kcptun/server
env CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_linux_mips github.com/xtaci/kcptun/client
env CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build -mod=vendor -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_linux_mips github.com/xtaci/kcptun/server

if $UPX; then upx -9 client_linux_mips* server_linux_mips*;fi
tar -cf kcptun-linux-mipsle-$VERSION.tar client_linux_mipsle server_linux_mipsle
${COMPRESS} -f kcptun-linux-mipsle-$VERSION.tar
$sum kcptun-linux-mipsle-$VERSION.tar.gz

tar -zcf kcptun-linux-mips-$VERSION.tar client_linux_mips server_linux_mips
${COMPRESS} -f kcptun-linux-mips-$VERSION.tar
$sum kcptun-linux-mips-$VERSION.tar.gz
