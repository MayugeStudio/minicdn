FROM golang:1.26 AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -v -o /usr/local/bin/edge ./...

FROM debian:bookworm-slim

COPY --from=builder /usr/local/bin/edge /usr/local/bin/edge
EXPOSE 5000
CMD ["edge"]

