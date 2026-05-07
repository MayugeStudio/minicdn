FROM golang:1.26

WORKDIR /usr/src/app

COPY go.mod ./
RUN go mod download

COPY . .
RUN go build -v -o /usr/local/bin/origin ./...

EXPOSE 9000

CMD ["origin"]

