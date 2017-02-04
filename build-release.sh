#!/bin/bash
unamestr=`uname`

inpath()
{   
    cmd=$1    path=$2    retval=1
    oldIFS=$IFS    IFS=":"

    for directory in $path
    do 
        if [ -x $directory/$cmd ]; then
            retval=0
        fi
    done
    IFS=$oldIFS
    return $retval
}

checkCmd()
{
    var="$1"
    if ! inpath $var $PATH; then
       return 1
    fi
}

main()
{
UPX=false
if hash upx 2>/dev/null; then
	UPX=true
fi

VERSION=`date -u +%Y%m%d`
LDFLAGS="-X main.VERSION=$VERSION -s -w"
GCFLAGS=""

OSES=(linux darwin windows freebsd)
ARCHS=(amd64 386)
for os in ${OSES[@]}; do
	for arch in ${ARCHS[@]}; do
		suffix=""
		if [ "$os" == "windows" ]
		then
			suffix=".exe"
		fi
		env CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_${os}_${arch}${suffix} github.com/xtaci/kcptun/client
		env CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_${os}_${arch}${suffix} github.com/xtaci/kcptun/server
		if $UPX; then upx -9 client_${os}_${arch}${suffix} server_${os}_${arch}${suffix};fi
		tar -zcf kcptun-${os}-${arch}-$VERSION.tar.gz client_${os}_${arch}${suffix} server_${os}_${arch}${suffix}
		$sum kcptun-${os}-${arch}-$VERSION.tar.gz
	done
done

# ARM
ARMS=(5 6 7)
for v in ${ARMS[@]}; do
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=$v go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_linux_arm$v  github.com/xtaci/kcptun/client
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=$v go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_linux_arm$v  github.com/xtaci/kcptun/server
done
if $UPX; then upx -9 client_linux_arm* server_linux_arm*;fi
tar -zcf kcptun-linux-arm-$VERSION.tar.gz client_linux_arm* server_linux_arm*
$sum kcptun-linux-arm-$VERSION.tar.gz

#MIPS32LE
env CGO_ENABLED=0 GOOS=linux GOARCH=mipsle go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_linux_mipsle github.com/xtaci/kcptun/client
env CGO_ENABLED=0 GOOS=linux GOARCH=mipsle go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_linux_mipsle github.com/xtaci/kcptun/server
env CGO_ENABLED=0 GOOS=linux GOARCH=mips go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o client_linux_mips github.com/xtaci/kcptun/client
env CGO_ENABLED=0 GOOS=linux GOARCH=mips go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o server_linux_mips github.com/xtaci/kcptun/server

if $UPX; then upx -9 client_linux_mips* server_linux_mips*;fi
tar -zcf kcptun-linux-mipsle-$VERSION.tar.gz client_linux_mipsle server_linux_mipsle
tar -zcf kcptun-linux-mips-$VERSION.tar.gz client_linux_mips server_linux_mips
$sum kcptun-linux-mipsle-$VERSION.tar.gz
$sum kcptun-linux-mips-$VERSION.tar.gz
exit
}


checkCmd "sha1sum"
case $? in
    0)sum="sha1sum"
      main
    ;;
    1)if checkCmd "shasum"; then
         sum="shasum"
	 main
      fi
      echo "Please install sha1sum or shasum."
      exit
    ;;
esac
