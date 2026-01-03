# Deployment Guide

This guide covers deploying ATLAS in various environments.

## Table of Contents

- [Local Development](#local-development)
- [Docker Deployment](#docker-deployment)
- [Production Deployment](#production-deployment)
- [Environment Variables](#environment-variables)
- [Database Setup](#database-setup)
- [Reverse Proxy](#reverse-proxy)

---

## Local Development

### Prerequisites

- Go 1.21+
- Git

### Setup

```bash
# Clone repository
git clone https://github.com/newthinker/atlas.git
cd atlas

# Install dependencies
go mod download

# Build
go build -o bin/atlas ./cmd/atlas

# Create configuration
cp configs/config.example.yaml config.yaml

# Run
./bin/atlas serve -c config.yaml --debug
```

### Development with Hot Reload

Use [air](https://github.com/cosmtrek/air) for automatic reloading:

```bash
# Install air
go install github.com/cosmtrek/air@latest

# Create .air.toml
cat > .air.toml << 'EOF'
[build]
cmd = "go build -o ./tmp/atlas ./cmd/atlas"
bin = "./tmp/atlas serve -c config.yaml --debug"
include_ext = ["go", "yaml", "html"]
exclude_dir = ["tmp", "vendor", ".git", ".worktrees"]
EOF

# Run with hot reload
air
```

---

## Docker Deployment

### Dockerfile

Create `Dockerfile`:

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o atlas ./cmd/atlas

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/atlas .
COPY --from=builder /app/internal/api/templates ./internal/api/templates

EXPOSE 8080

ENTRYPOINT ["./atlas"]
CMD ["serve", "-c", "/config/config.yaml"]
```

### Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  atlas:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - atlas-data:/data
    environment:
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
      - TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    restart: unless-stopped

  # Optional: TimescaleDB for production storage
  timescaledb:
    image: timescale/timescaledb:latest-pg15
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=atlas
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=atlas
    volumes:
      - timescale-data:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  atlas-data:
  timescale-data:
```

### Build and Run

```bash
# Build image
docker build -t atlas:latest .

# Run with docker-compose
docker-compose up -d

# View logs
docker-compose logs -f atlas

# Stop
docker-compose down
```

---

## Production Deployment

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| Memory | 512 MB | 2 GB |
| Disk | 1 GB | 10 GB (with history) |

### Systemd Service

Create `/etc/systemd/system/atlas.service`:

```ini
[Unit]
Description=ATLAS Trading Signal System
After=network.target

[Service]
Type=simple
User=atlas
Group=atlas
WorkingDirectory=/opt/atlas
ExecStart=/opt/atlas/bin/atlas serve -c /opt/atlas/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/atlas/data

# Environment
EnvironmentFile=/opt/atlas/.env

[Install]
WantedBy=multi-user.target
```

### Installation Steps

```bash
# Create user
sudo useradd -r -s /bin/false atlas

# Create directories
sudo mkdir -p /opt/atlas/{bin,data}

# Copy binary and config
sudo cp bin/atlas /opt/atlas/bin/
sudo cp config.yaml /opt/atlas/
sudo cp -r internal/api/templates /opt/atlas/

# Create environment file
sudo cat > /opt/atlas/.env << 'EOF'
TELEGRAM_BOT_TOKEN=your_token
TELEGRAM_CHAT_ID=your_chat_id
ANTHROPIC_API_KEY=your_api_key
EOF

# Set permissions
sudo chown -R atlas:atlas /opt/atlas
sudo chmod 600 /opt/atlas/.env

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable atlas
sudo systemctl start atlas

# Check status
sudo systemctl status atlas
sudo journalctl -u atlas -f
```

---

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TELEGRAM_BOT_TOKEN` | Telegram bot token for notifications | If Telegram enabled |
| `TELEGRAM_CHAT_ID` | Telegram chat ID for notifications | If Telegram enabled |
| `ANTHROPIC_API_KEY` | Claude API key for LLM features | If LLM enabled |
| `OPENAI_API_KEY` | OpenAI API key | If using OpenAI |
| `LIXINGER_API_KEY` | Lixinger API key for fundamentals | If Lixinger enabled |
| `FUTU_TRADE_PWD` | Futu trade password | If Futu broker enabled |

### Using .env File

Create `.env` in your project root:

```bash
TELEGRAM_BOT_TOKEN=123456:ABC-DEF...
TELEGRAM_CHAT_ID=-1001234567890
ANTHROPIC_API_KEY=sk-ant-...
```

Load in shell:

```bash
export $(cat .env | xargs)
./bin/atlas serve -c config.yaml
```

---

## Database Setup

### TimescaleDB (Recommended for Production)

```bash
# Install TimescaleDB
# Ubuntu/Debian
sudo apt install timescaledb-2-postgresql-15

# Create database
sudo -u postgres psql << 'EOF'
CREATE USER atlas WITH PASSWORD 'your_password';
CREATE DATABASE atlas OWNER atlas;
\c atlas
CREATE EXTENSION IF NOT EXISTS timescaledb;
EOF
```

Update `config.yaml`:

```yaml
storage:
  hot:
    dsn: "postgres://atlas:your_password@localhost:5432/atlas"
    retention_days: 90
  cold:
    type: localfs
    path: "/opt/atlas/data/archive"
```

### S3 Cold Storage

For S3-compatible storage (AWS S3, MinIO):

```yaml
storage:
  cold:
    type: s3
    s3:
      bucket: "atlas-archive"
      endpoint: "https://s3.amazonaws.com"  # or MinIO endpoint
      region: "us-east-1"
      access_key: "${AWS_ACCESS_KEY_ID}"
      secret_key: "${AWS_SECRET_ACCESS_KEY}"
      prefix: "atlas/"
```

---

## Reverse Proxy

### Nginx

Create `/etc/nginx/sites-available/atlas`:

```nginx
server {
    listen 80;
    server_name atlas.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name atlas.example.com;

    ssl_certificate /etc/letsencrypt/live/atlas.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/atlas.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable the site:

```bash
sudo ln -s /etc/nginx/sites-available/atlas /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### Caddy

Create `Caddyfile`:

```
atlas.example.com {
    reverse_proxy localhost:8080
}
```

---

## Health Checks

ATLAS exposes a health endpoint:

```bash
curl http://localhost:8080/api/health
# {"status":"ok"}
```

For Docker health checks:

```yaml
services:
  atlas:
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

---

## Logging

Logs are written to stdout in JSON format. Use your preferred log aggregation tool:

```bash
# View logs with jq
./bin/atlas serve -c config.yaml 2>&1 | jq .

# With systemd
journalctl -u atlas -f | jq .
```

### Log Levels

Set debug mode for verbose logging:

```bash
./bin/atlas serve -c config.yaml --debug
```

---

## Backup

### Configuration Backup

```bash
# Backup config and environment
tar -czf atlas-config-$(date +%Y%m%d).tar.gz \
    /opt/atlas/config.yaml \
    /opt/atlas/.env
```

### Data Backup

```bash
# Backup archive data
tar -czf atlas-data-$(date +%Y%m%d).tar.gz \
    /opt/atlas/data/archive/

# For TimescaleDB
pg_dump -U atlas atlas > atlas-db-$(date +%Y%m%d).sql
```

---

## Troubleshooting

### Common Issues

**Port already in use:**
```bash
# Find process using port 8080
lsof -i :8080
# Kill it or change port in config
```

**Permission denied:**
```bash
# Fix file permissions
sudo chown -R atlas:atlas /opt/atlas
```

**Cannot connect to database:**
```bash
# Check PostgreSQL is running
sudo systemctl status postgresql
# Check connection
psql -U atlas -h localhost -d atlas
```

**Telegram notifications not working:**
```bash
# Test bot token
curl "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe"
# Test sending message
curl "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage?chat_id=${TELEGRAM_CHAT_ID}&text=test"
```
