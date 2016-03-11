#!/bin/sh

gox github.com/xtaci/kcptun/client github.com/xtaci/kcptun/server
tar -zcf kcptun-linux.tar.gz client_linux_* server_linux_*
tar -zcf kcptun-darwin.tar.gz client_darwin_* server_darwin_*
tar -zcf kcptun-windows.tar.gz client_windows_* server_windows_*
