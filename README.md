# kcptun
TCP流转换为KCP+UDP流(AES加密)，工作示意图:        
原client -> kcptun client ->  kcptun server -> 原server

# 安装
1. 服务端: go get github.com/xtaci/kcptun/server;  server 
2. 客户端: go get github.com/xtaci/kcptun/client;  client

执行 client -h , server -h 查看帮助

# 举例
1. openvpn client -> kcptun client -> kcptun server -> openvpn server
2. ssh client -> kcptun client -> kcptun server -> sshd
2. browser socks5 proxy(pac) -> kcptun client -> kcptun server -> socks5 server

# 贡献
欢迎短小精干的PR
