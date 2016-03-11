# kcptun
TCP流转换为KCP+UDP流(AES加密)，工作示意图:        
原client -> kcptun client ->  kcptun server -> 原server

# 安装
1. 服务端: go get github.com/xtaci/kcptun/server;  server 
2. 客户端: go get github.com/xtaci/kcptun/client;  client

# 使用
```A
ubuntu@gateway:~$ client -h
NAME:
   kcptun - kcptun client

USAGE:
   client [global options] command [command options] [arguments...]

VERSION:
   1.0

COMMANDS:
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --localaddr, -l ":12948"	local listen addr:
   --remoteaddr, -r "vps:29900"	kcp server addr
   --key "it's a secrect"	key for communcation, must be the same as kcptun server
   --help, -h			show help
   --version, -v		print the version
```

```
ubuntu@gateway:~$ server -h
NAME:
   kcptun - kcptun server

USAGE:
   server [global options] command [command options] [arguments...]

VERSION:
   1.0

COMMANDS:
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --listen, -l ":29900"		kcp server listen addr:
   --target, -t "127.0.0.1:12948"	target server addr
   --key "it's a secrect"		key for communcation, must be the same as kcptun client
   --help, -h				show help
   --version, -v			print the version
```

# 举例
1. openvpn client -> kcptun client -> kcptun server -> openvpn server
2. ssh client -> kcptun client -> kcptun server -> sshd
2. browser socks5 proxy(pac) -> kcptun client -> kcptun server -> socks5 server

# 贡献
欢迎短小精干的PR
