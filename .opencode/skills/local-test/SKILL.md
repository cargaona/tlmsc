---
name: local-test
description: Build and run tlmsc bot locally with docker compose using the test bot (testtybot)
---

## Local Test

Use this workflow to test the bot locally before deploying to Kubernetes.

## Prerequisites

- Docker and docker compose installed
- `.env` file in the project root with test bot credentials (testtybot)
- The `.env` file should NOT be committed (it's in .gitignore)

## Steps

### 1. Ensure .env exists

The `.env` file must contain at minimum:

```
TELEGRAM_BOT_TOKEN=<testtybot token>
DEBUG=true
QOBUZ_EMAIL=...
QOBUZ_PASSWORD=...
DEEZER_ARL=...
```

Copy from `.env.example` if needed. The test bot username is `@elcinebot` (testtybot).

### 2. Build and run

```bash
docker compose up -d --build
```

### 3. Check logs

```bash
docker logs tlmsc-bot
```

Look for:
- `Bot @elcinebot started successfully`
- `setMyCommands` response `"ok":true`
- `Bot is running`

### 4. Test in Telegram

Open `@elcinebot` in Telegram and test the changes.

### 5. Stop when done

```bash
docker compose down
```

## Notes

- The local container name is `tlmsc-bot` — if it conflicts with an existing container, remove it first: `docker rm -f tlmsc-bot`
- The production bot (`@ripripmusicbot`) runs in K8s and uses a different token. Local testing does NOT affect production.
- Staging data uses a docker volume (`staging_data`), not a host mount.
