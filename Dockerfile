FROM golang:latest
MAINTAINER xtaci <daniel820313@gmail.com>
RUN go get github.com/xtaci/kcptun/client
RUN go get github.com/xtaci/kcptun/server
EXPOSE 29900/udp
EXPOSE 12948
