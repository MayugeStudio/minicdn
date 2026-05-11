FROM golang:1.26 AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -v -o /usr/local/bin/origin ./...

FROM debian:bookworm-slim

COPY --from=builder /usr/local/bin/origin /usr/local/bin/origin
EXPOSE 9000
CMD ["origin"]

