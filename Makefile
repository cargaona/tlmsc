.PHONY: help build run stop logs clean test fmt lint dev-setup docker-build docker-up docker-down

help:
	@echo "TeleMusic - Telegram Bot for Album Downloads"
	@echo ""
	@echo "Available commands:"
	@echo "  make build          Build the bot binary locally"
	@echo "  make run            Run the bot locally (requires dependencies)"
	@echo "  make stop           Stop the bot"
	@echo "  make logs           Show bot logs"
	@echo "  make clean          Clean build artifacts"
	@echo "  make test           Run tests"
	@echo "  make fmt            Format code"
	@echo "  make lint           Run linter"
	@echo "  make dev-setup      Setup local development environment"
	@echo "  make docker-build   Build Docker image"
	@echo "  make docker-up      Start Docker containers"
	@echo "  make docker-down    Stop Docker containers"
	@echo "  make docker-logs    Show Docker logs"
	@echo "  make docker-shell   Open shell in bot container"

build:
	go build -o telemusic-bot ./cmd/bot

run: build
	./telemusic-bot

stop:
	pkill -f "telemusic-bot" || true

logs:
	docker logs -f telemusic-bot

clean:
	rm -f telemusic-bot
	go clean -cache

test:
	go test -v ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run ./... || go vet ./...

dev-setup:
	@echo "Installing dependencies..."
	go mod download
	pip install streamrip beets
	@echo "Creating data directories..."
	mkdir -p /data/staging /data/music
	@echo "Setup complete! Copy .env.example to .env and configure your bot token"

docker-build:
	docker-compose build

docker-up:
	docker-compose up -d
	@echo "Bot started! Check logs with: make docker-logs"

docker-down:
	docker-compose down

docker-logs:
	docker logs -f telemusic-bot

docker-shell:
	docker exec -it telemusic-bot /bin/bash

docker-setup-streamrip:
	docker exec -it telemusic-bot bash /app/scripts/setup-streamrip.sh

.PHONY: docker-setup-streamrip
