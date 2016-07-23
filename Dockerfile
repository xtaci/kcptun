FROM alpine:latest
MAINTAINER xtaci <daniel820313@gmail.com>
WORKDIR /bin
RUN wget https://github.com/xtaci/kcptun/releases/download/v20160719/kcptun-linux-amd64-20160719.tar.gz
RUN tar -zvxf kcptun-linux-amd64-20160719.tar.gz
EXPOSE 29900/udp
EXPOSE 12948
