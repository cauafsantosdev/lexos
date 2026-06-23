FROM golang:1.26.4-alpine

WORKDIR /app

RUN apk add --no-cache git build-base

RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./

RUN go mod download

COPY . .

EXPOSE 8000

CMD ["air", "--build.cmd", "go build -o tmp/main ./cmd/server", "--build.bin", "./tmp/main"]