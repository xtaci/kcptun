#!/bin/sh

gox github.com/xtaci/kcptun/client github.com/xtaci/kcptun/server
tar -zcf kcptun-linux-x86.tar.gz client_linux_386 server_linux_386
tar -zcf kcptun-darwin-x86.tar.gz client_darwin_386 server_darwin_386
tar -zcf kcptun-windows-x86.tar.gz client_windows_386.exe server_windows_386.exe

tar -zcf kcptun-linux-amd64.tar.gz client_linux_amd64 server_linux_amd64
tar -zcf kcptun-darwin-amd64.tar.gz client_darwin_amd64 server_darwin_amd64
tar -zcf kcptun-windows-amd64.tar.gz client_windows_amd64.exe server_windows_amd64.exe

tar -zcf kcptun-linux-arm.tar.gz client_linux_arm server_linux_arm
