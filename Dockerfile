FROM golang:alpine
MAINTAINER xtaci <daniel820313@gmail.com>
RUN apk update && \
    apk upgrade && \
    apk add git
RUN go get github.com/xtaci/kcptun/client && go get github.com/xtaci/kcptun/server
EXPOSE 29900/udp
EXPOSE 12948
