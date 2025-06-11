FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o mc-webhook main.go

FROM alpine:latest

WORKDIR /

COPY --from=builder /app/mc-webhook .

RUN apk add --no-cache docker-cli

ENTRYPOINT ["./mc-webhook"]
