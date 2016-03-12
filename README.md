# kcptun
TCP流转换为KCP+UDP流(AES加密)，用于在高丢包环境中，TCP降速严重的问题，工作示意图:      
```
+-----------+--------------+----------------+------------+
|           |              |                |            |
|  Client +--> KCP Client +--> KCP Server +----> Server  |
|           |              |                |            |
+-----------+--------------+----------------+------------+
```

kcptun的用途是: ***端口转发***

kcptun客户端和服务端分别只有一个main.go文件，非常简单，也方便自己修改。      

# 安装前的准备
注意，请确保默认服务器端UDP端口 ```29900``` 开启，防火墙允许UDP包通过。   (端口可以通过命令行参数调整，不要忘记修改对应的防火墙规则。)

# 基于二进制的安装 (使用简单)
在release中下载对应平台的版本， 执行 client -h 和server -h 查看详细使用方法.        
我们以加速ssh访问为例示范使用方法如下：         

1. 假定服务器IP为:```xxx.xxx.xxx.xxx```

2. 在服务器端开启socks5      (监听127.0.0.1:8080端口)
```ssh -D 127.0.0.1:8080 ubuntu@localhost```   

3. 在服务器启动kcp server:     
```server -t "127.0.0.1:8080"  ```     // 所有数据包转发到sshd进程的socks 8080端口           

 ----------------------------  分割线，上面是服务器，下面是客户端  ----------------------------  
4. 在本地启动kcp client:          
```client -r "xxx.xxx.xxx.xxx:29900"   ```    // 连接到kcp server，默认server端口是29900           

5. 浏览器就可以连接12948端口的socks代理了   // 默认client的端口是12948

# 基于源码的安装  (方便使用最新版本)
## 预备条件:       
1. 安装好```golang```       
2. 设置好```GOPATH```  以及```PATH=$PATH:$GOPATH/bin``` (例如: ```export GOPATH=/home/ubuntu;  export PATH=$PATH:$GOPATH/bin```), 最好放到.bashrc 或 .zshrc中 

## 安装命令
1. 服务端: ```go get github.com/xtaci/kcptun/server;  server```        
![server](server.gif)      

2. 客户端: ```go get github.com/xtaci/kcptun/client;  client```      
![client](client.gif)    


# 使用案例
1. openvpn client -> kcptun client -> kcptun server -> openvpn server
2. ssh client -> kcptun client -> kcptun server -> sshd
2. browser socks5 -> kcptun client -> kcptun server -> ssh -D socks5 server

# 常见问题
Q: client/server都启动了，但无法传输数据，服务器显示了stream open        
A: 先杀掉client/server，然后重新启动就能解决绝大部分的问题             

Q: client/server都启动了，但服务器没有收到任何数据包也没有stream open          
A: 某些IDC默认屏蔽了UDP协议，需要在防火墙中打开对应的端口

Q: 出现不明原因降速严重，可能有50%丢包         
A: 可能该端口被运营商限制，更换一个端口就能解决        

# 贡献
欢迎短小精干的PR
