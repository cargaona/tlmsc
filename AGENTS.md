# AGENTS.md

## Project Overview

TeleMusic (tlmsc) is a Go Telegram bot that searches, downloads, and imports albums
into a beets music library. It shells out to the `rip` CLI (streamrip) for downloads
and `beet` CLI for library imports. Single external Go dependency: telegram-bot-api v5.

Module: `tlmsc` | Go 1.25 | One integration test (`internal/streamrip/client_test.go`,
skips without the `rip` CLI or credentials).

## Project Structure

```
cmd/bot/main.go              # Entrypoint: init, wire components, signal handling
internal/
  config/config.go           # Env-based configuration (no config files)
  cover/cover.go             # Deezer cover art fetching via HTTP API
  download/queue.go          # Channel-based job queue
  download/worker.go         # Download worker with retry/source fallback
  streamrip/client.go        # Exec wrapper for `rip` CLI
  streamrip/parser.go        # Parse search/progress output (JSON + line-based)
  streamrip/types.go         # Domain types: Album, Progress, DownloadJob
  telegram/bot.go            # Bot struct, polling, message send/edit helpers
  telegram/handlers.go       # Command & callback handlers (/search, /import, etc.)
scripts/entrypoint.sh        # Docker entrypoint: streamrip config + credentials
Dockerfile                   # Multi-stage: Go builder + Python runtime (streamrip, beets)
docker-compose.yaml          # Single-service compose for local dev
```

## Build / Run / Test Commands

The dev shell is [devenv](https://devenv.sh) (same as `kube-server`, `reet`). It puts
Go, `rip`, and `beet` on PATH and points `STAGING_PATH` / `MUSIC_LIBRARY_PATH` /
`BEETSDIR` at `.devenv-state/` so local runs never touch the real library.

```bash
direnv allow          # once; or `devenv shell` manually
tlhelp                # command reference
tlcheck               # verify rip/beet + credentials (incl. live provider auth)
tlrun                 # go run ./cmd/bot
```

```bash
# Build binary
make build                    # or: go build -o telemusic-bot ./cmd/bot

# Run locally (requires streamrip + beets installed, .env configured)
make run

# Run all tests
make test                     # or: go test -v ./...

# Run a single test
go test -v -run TestFunctionName ./internal/package/

# Format code
make fmt                      # or: go fmt ./...

# Lint (falls back to go vet if golangci-lint not installed)
make lint                     # or: golangci-lint run ./... || go vet ./...

# Docker
make docker-build             # Build image
make docker-up                # Start container
make docker-down              # Stop container
make docker-shell             # Shell into running container
```

## CI/CD

- GitHub Actions workflow on `develop` branch: builds Docker image and pushes to
  `ghcr.io/cargaona/tlmsc:latest`. No CI test or lint step.
- Kubernetes deployment reads image from GHCR. Beets config delivered via ConfigMap.

## Code Style Guidelines

### Imports

Two-group style separated by blank line:
1. Standard library
2. Third-party and internal (`tlmsc/internal/...`) together

```go
import (
    "fmt"
    "os"
    "strings"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "tlmsc/internal/cover"
    "tlmsc/internal/streamrip"
)
```

### Formatting

- Standard `gofmt` formatting. Tabs for indentation.
- Run `go fmt ./...` before committing.

### Naming Conventions

- **Exported**: PascalCase (`NewBot`, `HandleSearch`, `ParseSearchJSON`)
- **Unexported**: camelCase (`getEnv`, `parseAlbumLine`, `executeDownload`)
- **Struct fields**: PascalCase exported (`TelegramToken`), camelCase unexported (`debug`)
- **Constants**: camelCase unexported (`resultsPerPage = 10`)
- **Acronyms**: Keep capitalized (`ID`, `URL`, `ChatID`)
- **Constructors**: `NewX` pattern returning pointer (`NewBot`, `NewClient`, `NewQueue`)
- **Packages**: Single lowercase word matching directory name

### Type Definitions

- Plain structs, no embedded interfaces. JSON struct tags for API deserialization only.
- Pointer receivers for all methods on stateful structs (`*Bot`, `*Handlers`, `*Worker`).
- Constructor `NewX(*args) *Type` or `NewX(*args) (*Type, error)` if initialization can fail.
- Function types for callbacks: `type CommandHandler func(...)`, `type DownloadCallback func(...)`.
- No interfaces defined -- all concrete types. No dependency injection.
- Channel-based concurrency for the download queue (buffered channel + mutex).

### Error Handling

- Wrap errors with context: `fmt.Errorf("description: %w", err)`
- No custom error types or sentinel errors.
- Early return on error (guard clause pattern).
- Fatal errors in `main()`: `fmt.Println(msg)` + `os.Exit(1)` (not `log.Fatal`).
- Optional operations (e.g., cover art) silently return zero values on failure.
- Debug-gated logging: only log non-critical errors when `debug` is true.

```go
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

### Logging

- No `log` package. All logging via `fmt.Printf` / `fmt.Println`.
- Tag-prefixed format: `fmt.Printf("[tag] message: %v\n", value)`
- Tags: `[telegram]`, `[download]`, `[streamrip]`, `[handlers]`, `[worker]`, `[import]`
- Debug messages gated behind `if h.debug { ... }` or `if w.debug { ... }`

### Comments

- Exported functions: doc comment starting with function name (Go convention).
- Unexported functions: doc comment when purpose is non-obvious.
- Inline comments sparingly for clarification.
- No block comments (`/* */`).

### External CLI Execution

- `streamrip` package wraps the `rip` CLI via `os/exec.Command`.
- `handlers.go` shells out to `beet import` for library imports.
- stderr is captured separately (streamrip writes progress to stderr).
- Always set working directory and staging path explicitly.

### Concurrency

- `context.Context` for cancellation, `sync.WaitGroup` for goroutine lifecycle.
- OS signal handling (`SIGINT`, `SIGTERM`) for graceful shutdown.
- Download queue: buffered channel consumed by a single worker goroutine.
- Per-chat search results stored in package-level map (no mutex -- single-writer assumed).

### Configuration

- All config via environment variables, loaded in `internal/config/config.go`.
- Key vars: `TELEGRAM_BOT_TOKEN`, `STAGING_PATH`, `BEETSDIR`, `DEBUG`,
  `QOBUZ_EMAIL`, `QOBUZ_PASSWORD`, `DEEZER_ARL`.
- Defaults provided for paths; token is required (exits if missing).

## Dependencies

- **Go**: `github.com/go-telegram-bot-api/telegram-bot-api/v5` (only external dep)
- **Runtime**: Python 3.11 with `streamrip` and `beets` (pip), `ffmpeg`
- **External CLIs**: `rip` (streamrip), `beet` (beets) -- invoked via `os/exec`
