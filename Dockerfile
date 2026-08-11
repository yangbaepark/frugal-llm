# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download dependencies first for caching
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary with stripped debug symbols
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o frugal-llm ./cmd/proxy-server/main.go

# Minimal runtime stage (Scratch / Distroless)
FROM alpine:latest AS runner

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/frugal-llm .

EXPOSE 8080

ENTRYPOINT ["/app/frugal-llm"]
