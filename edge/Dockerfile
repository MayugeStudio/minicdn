FROM golang:1.26 AS builder
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=$HTTP_PROXY HTTPS_PROXY=$HTTPS_PROXY NO_PROXY=$NO_PROXY

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -v -o /usr/local/bin/edge ./...

FROM debian:bookworm-slim

COPY --from=builder /usr/local/bin/edge /usr/local/bin/edge
EXPOSE 5000
CMD ["edge"]

