FROM golang:1.14.9-alpine3.11 as builder
MAINTAINER xtaci <daniel820313@gmail.com>
ENV GO111MODULE=on
RUN apk update && \
    apk upgrade && \
    apk add git gcc libc-dev linux-headers
RUN go get -ldflags "-X main.VERSION=$(date -u +%Y%m%d) -s -w" github.com/xtaci/kcptun/client && go get -ldflags "-X main.VERSION=$(date -u +%Y%m%d) -s -w" github.com/xtaci/kcptun/server

FROM alpine:3.11
RUN apk add --no-cache iptables
COPY --from=builder /go/bin /bin
EXPOSE 29900/udp
EXPOSE 12948
