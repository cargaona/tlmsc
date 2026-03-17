# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the bot
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tlmsc-bot ./cmd/bot

# Runtime stage
FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Install Python packages
RUN pip install --no-cache-dir streamrip beets

# Create data directories
RUN mkdir -p /data/staging

# Copy binary from builder
COPY --from=builder /app/tlmsc-bot /app/tlmsc-bot

# Copy entrypoint script
COPY scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Expose staging directory
VOLUME ["/data/staging"]

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ps aux | grep tlmsc-bot | grep -v grep || exit 1

# Set entrypoint and command
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/tlmsc-bot"]
