# TLMSC - Telegram Music Search & Downloader

A lightweight Go-based Telegram bot that searches for albums on Qobuz and Deezer, and downloads them via streamrip to a staging folder. 

Music library management is handled by a separate process. You can use beets with drop2beets to monitor the download folder`

## Features

- 🔍 **Album Search** - Search Qobuz and Deezer with pagination (10 results/page, up to 50 total)
- ⬇️ **Streamlined Downloads** - Direct download to staging folder via `rip id`
- 🔄 **Automatic Fallback** - If Qobuz fails, automatically retries with Deezer
- 📱 **Telegram Integration** - Inline keyboards, pagination, real-time logging
- 🐳 **Docker-Ready** - Simple Docker Compose setup for quick deployment
- 🔐 **Environment-Based Config** - Credentials via env vars (no hardcoding)

## Architecture

```
┌─────────────────────────────────────────┐
│         TLMSC Container                 │
│  (Telegram Bot + Streamrip)             │
│                                         │
│  ✓ Telegram bot (/start, /search)      │
│  ✓ Album search (Qobuz + Deezer)       │
│  ✓ Download to staging                 │
│  ✓ Completion logging                  │
└────────────┬──────────────────────────┘
             │
             ▼ (shared volume)
        /data/staging/
```

## Prerequisites

### Container
- Docker & Docker Compose
- Telegram Bot Token (get from @BotFather)
- Qobuz or Deezer account credentials


## Quick Start

### 1. Setup

```bash
# Clone and setup
git clone https://github.com/YOUR_USERNAME/tlmsc.git
cd tlmsc
cp .env.example .env
```

### 2. Configure Credentials

Edit `.env` with your credentials:

```env
TELEGRAM_BOT_TOKEN=your_token_here
QOBUZ_EMAIL=your_email@example.com
QOBUZ_PASSWORD=your_token_or_password
DEEZER_ARL=your_arl_token_here
DEBUG=true
```

**Getting Credentials:**
- **Telegram:** Message @BotFather on Telegram
- **Qobuz:** MD5 hash of your password (or token if pre-hashed)
- **Deezer:** Your browser cookie `arl` token

### 3. Start the Bot

```bash
docker compose up -d
```

Logs should show:
```
🎵 TLMSC Entrypoint - Initializing...
✅ Streamrip configuration generated
🚀 Starting TLMSC bot...
Bot @yourbot started successfully
```

### 4. Use the Bot

In Telegram:

```
/start                          - Welcome message
/search kiss                    - Search for albums (pagination ready)
/queue                          - Show download status
```

Select an album → bot downloads to `/data/staging`

## Usage

### Telegram Commands

| Command | Example | Result |
|---------|---------|--------|
| `/start` | `/start` | Welcome message with available commands |
| `/search` | `/search pez los orfebres` | Search results with 10-per-page pagination |
| `/queue` | `/queue` | Show currently downloading album |

### Download Flow

1. User sends `/search album_name`
2. Bot searches Qobuz first, then Deezer if needed
3. Results shown with pagination (10 per page)
4. User clicks album button to download
5. Bot: `[download] Completed: Album Title`

### Automatic Retry

If Qobuz download fails:
- Automatically retries with Deezer
- User sees seamless experience
- If both fail, user is notified

## Configuration

### Environment Variables

```env
# Required
TELEGRAM_BOT_TOKEN=your_bot_token

# Streamrip Credentials
QOBUZ_EMAIL=user@example.com
QOBUZ_PASSWORD=md5_hash_or_token
DEEZER_ARL=arl_token

# Optional (defaults shown)
STAGING_PATH=/data/staging
DEBUG=false
```

### Docker Compose

See `docker-compose.yaml` for volume mounts and networking options.


## Development

### Building locally

```bash
go build -o tlmsc-bot ./cmd/bot
```

### Testing

```bash
go test ./...
```

### Running without Docker

```bash
export TELEGRAM_BOT_TOKEN=your_token
export QOBUZ_EMAIL=user@example.com
export QOBUZ_PASSWORD=your_hash
export DEEZER_ARL=your_arl
mkdir -p /data/staging
go run ./cmd/bot
```


### Using published images

```bash
# Pull latest
docker pull ghcr.io/YOUR_USERNAME/tlmsc:develop

# Or in docker-compose.yaml
image: ghcr.io/cargaona/tlmsc:develop
```

## Known Limitations

- Search results stored in memory (lost on restart)
- No download history persistence
- Sequential downloads (one at a time)
- No user authentication/rate limiting

## Future Improvements

- [ ] Persistent download history
- [ ] Multiple concurrent downloads
- [ ] Playlist support
- [ ] Additional music sources 
- [ ] More detailed progress updates

## License

GPL-3.0 (same as streamrip)
