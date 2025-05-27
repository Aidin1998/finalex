FROM golang:1.20 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o market-maker-bot ./...

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/market-maker-bot .

CMD ["./market-maker-bot"]