# ExpenseLog – Self-Hosted Deployment

Personal instance of ExpenseLog running on a homeserver with Docker Compose, Cloudflare Tunnel, and PostgreSQL.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Homeserver (Proxmox)                     │
│                                                                 │
│  ┌─────────────┐    ┌──────────────┐    ┌───────────────────┐   │
│  │  cloudflared │───▶│  expenselog  │───▶│    postgres        │   │
│  │  (tunnel)    │    │  (Go :8080)  │    │  (17-alpine)      │   │
│  └─────────────┘    └──────────────┘    │                   │   │
│                            ▲             │  • expenselog     │   │
│  ┌─────────────┐           │             │  • expenselog_bot │   │
│  │ telegram-bot│───────────┘             └───────────────────┘   │
│  │ (Node :3000)│                                ▲               │
│  └──────┬──────┘                                │               │
│         │           ┌──────────────┐             │               │
│         └──────────▶│  ai-parser   │             │               │
│                     │ (Gemini:3000)│             │               │
│                     └──────────────┘             │               │
│                                                  │               │
│  ┌─────────────┐                                 │               │
│  │   backup    │─────────────────────────────────┘               │
│  │ (on-demand) │                                                 │
│  └─────────────┘                                                 │
└─────────────────────────────────────────────────────────────────┘
                           │
                    Cloudflare Tunnel
                           │
                ┌──────────▼──────────┐
                │ me.expenselog.com.ar │
                └─────────────────────┘
```

## Prerequisites

- Docker Engine 24+ with Compose v2
- Cloudflare account with `expenselog.com.ar` domain
- Cloudflare Tunnel token (Zero Trust → Tunnels → Create)
- (Optional) Gemini API key for AI parser
- (Optional) Telegram bot token from @BotFather

## Quick Start

```bash
# 1. Clone and switch to the homeserver branch
git clone https://github.com/joaquingonzalezzanotti/ExpenseLog.git
cd ExpenseLog
git checkout docker/homeserver

# 2. Create environment files
cd docker
cp .env.example .env
cp .env.bot.example .env.bot       # optional: only if using bot
cp .env.parser.example .env.parser  # optional: only if using parser

# 3. Edit .env files with your values
#    At minimum, set: POSTGRES_PASSWORD, STORAGE_PASS, BOOTSTRAP_EMAIL,
#    BOOTSTRAP_PASSWORD, CLOUDFLARE_TUNNEL_TOKEN

# 4. Start the core stack (app + postgres + tunnel)
docker compose up -d

# 5. (Optional) Add bot and/or parser
docker compose --profile bot --profile parser up -d
```

## Common Commands

```bash
# View logs
docker compose logs -f expenselog
docker compose logs -f postgres

# Check health status
docker compose ps

# Restart a specific service
docker compose restart expenselog

# Rebuild after code changes
docker compose build expenselog
docker compose up -d expenselog

# Run a backup
docker compose --profile tools run --rm backup

# Restore from backup
docker compose --profile tools run --rm \
  backup sh /restore.sh expenselog /backups/expenselog-YYYYMMDD-HHMMSS.dump

# Stop everything
docker compose --profile bot --profile parser down

# Stop and remove volumes (DESTRUCTIVE)
docker compose --profile bot --profile parser down -v
```

## Profiles

| Profile   | Services                 | When to use                          |
|-----------|--------------------------|--------------------------------------|
| (default) | postgres, expenselog, cloudflared | Always – core stack           |
| `bot`     | + telegram-bot           | When using Telegram bot              |
| `parser`  | + ai-parser              | When using AI receipt parsing        |
| `tools`   | backup (on-demand)       | For backup/restore operations        |

## Cloudflare Tunnel Setup

1. Go to [Cloudflare Zero Trust](https://one.dash.cloudflare.com/) → Networks → Tunnels
2. Create a new tunnel (type: Cloudflared)
3. Copy the tunnel token and paste it into `.env` as `CLOUDFLARE_TUNNEL_TOKEN`
4. In the tunnel's **Public Hostname** tab, add:
   - Subdomain: `me` | Domain: `expenselog.com.ar` | Service: `http://expenselog:8080`
5. Save and deploy

## Data Migration

To import data from Neon (Railway's PostgreSQL), see [migrate-from-neon.md](./migrate-from-neon.md).

## Updating

To pull changes from `main` and rebuild:

```bash
# Fetch latest changes
git fetch origin main

# Merge into homeserver branch
git merge origin/main

# Rebuild and restart
docker compose build
docker compose --profile bot --profile parser up -d
```

## Backups

### Manual backup
```bash
docker compose --profile tools run --rm backup
```

### Automated daily backup (host crontab)
```bash
# Add to crontab -e on the host:
0 3 * * * cd /path/to/ExpenseLog && docker compose -f docker/compose.yaml --profile tools run --rm backup >> /var/log/expenselog-backup.log 2>&1
```

### Restore
```bash
docker compose --profile tools run --rm \
  backup sh /restore.sh expenselog /backups/expenselog-YYYYMMDD-HHMMSS.dump
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `expenselog` not healthy | Check `docker compose logs expenselog` – likely missing env vars |
| Can't connect to PostgreSQL | Verify `STORAGE_URL`, `STORAGE_USER`, `STORAGE_PASS` match `POSTGRES_*` |
| Tunnel not working | Verify `CLOUDFLARE_TUNNEL_TOKEN` and public hostname config in CF dashboard |
| Bot conflicts (409) | You need a separate bot token from @BotFather for the homeserver |
| OCR not working | Check that `tesseract-ocr-spa` is installed (it is in the hardened images) |
| Prisma errors on bot start | Run `docker compose --profile bot run --rm telegram-bot npx prisma db push` |
