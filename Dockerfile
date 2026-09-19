# Build stage (Uses BuildKit BUILDPLATFORM to compile natively for TARGETARCH)
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Download dependencies first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and assets
COPY . .

# Build statically linked binary with stripped debug symbols for the target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o frugal-llm ./cmd/proxy-server/main.go

# Minimal runtime stage
FROM alpine:latest AS runner

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary and runtime assets (config & prompt templates)
COPY --from=builder /app/frugal-llm .
COPY --from=builder /app/config.yaml .
COPY --from=builder /app/templates ./templates

EXPOSE 8080

ENTRYPOINT ["/app/frugal-llm"]
