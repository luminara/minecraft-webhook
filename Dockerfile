FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --platform=$BUILDPLATFORM \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o mc-webhook main.go

FROM --platform=$TARGETPLATFORM alpine:latest

WORKDIR /

COPY --from=builder /app/mc-webhook .

RUN apk add --no-cache docker-cli

ENTRYPOINT ["./mc-webhook"]
