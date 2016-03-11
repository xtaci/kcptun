# kcptun
TCP流转换为KCP+UDP流(AES加密)，工作示意图:        
原client -> kcptun client ->  kcptun server -> 原server

# 安装
1. 服务端: go get github.com/xtaci/kcptun/server;  server 
2. 客户端: go get github.com/xtaci/kcptun/client;  client

# 使用 -- 以ssh tunnel为例
kcptun 服务端
![server](server.gif)
kcptun 客户端
![client](client.gif)

客户端和服务端启动后，使用ssh -D 连接kcptun客户端，即可发起socks通信.

# 举例
1. openvpn client -> kcptun client -> kcptun server -> openvpn server
2. ssh client -> kcptun client -> kcptun server -> sshd
2. browser socks5 proxy(pac) -> kcptun client -> kcptun server -> socks5 server

# 贡献
欢迎短小精干的PR
