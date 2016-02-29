# kcptun
TCP流转换为KCP+UDP流，工作示意图:        
原client -> kcptun client ->  kcptun server -> 原server

# 安装
1. 服务端: go get github.com/xtaci/kcptun/server;  server 
2. 客户端: go get github.com/xtaci/kcptun/client;  client

执行 client -h , server -h 查看帮助
