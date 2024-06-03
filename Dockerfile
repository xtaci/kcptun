FROM golang:1.21.0-alpine3.18 as builder
MAINTAINER xtaci <daniel820313@gmail.com>
LABEL org.opencontainers.image.source=https://github.com/xtaci/kcptun
ENV GO111MODULE=on
RUN apk add git
RUN git clone https://github.com/xtaci/kcptun.git
RUN cd kcptun && \
	go build -mod=vendor -ldflags "-X main.VERSION=$(date -u +%Y%m%d) -s -w" -o /client github.com/xtaci/kcptun/client && \
	go build -mod=vendor -ldflags "-X main.VERSION=$(date -u +%Y%m%d) -s -w" -o /server github.com/xtaci/kcptun/server

FROM alpine:3.18
RUN apk add --no-cache iptables
COPY --from=builder /client /bin
COPY --from=builder /server /bin
EXPOSE 29900/udp
EXPOSE 12948
