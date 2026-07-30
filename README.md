# t1ms-go

[![codecov](https://codecov.io/github/TheKigen/t1ms-go/graph/badge.svg?token=HYH4LKATGX)](https://codecov.io/github/TheKigen/t1ms-go)

A Starsiege: Tribes (aka Tribes 1) master server written in Go.

## Features

- UDP master server implementing the Tribes 1 protocol
- Discovers game servers by querying configured peer master servers
- Validates game servers by querying them directly
- Responds to Tribes client server-list requests
- Web interface with a Tribes-styled server browser (servers, masters, setup guide)
- REST API serving server/master/stats data in JSON and XML
- API endpoint to add game servers with API key auth, rate limiting, and private IP filtering
- Hot-reloadable XML configuration (SIGHUP or console command)
- Light and dark theme support in the web frontend

## Requirements

- Go 1.25 or later

## Building

```sh
go build -trimpath -ldflags="-s -w" ./cmd/t1ms/
```

## Usage

```sh
# Run with default config path (config.xml)
./t1ms

# Specify a config file
./t1ms -c /etc/t1ms/config.xml

# Run in service/daemon mode (no interactive console)
./t1ms -k -c /etc/t1ms/config.xml
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-c` | Config file path | `config.xml` |
| `-k` | Service/daemon mode (disables interactive console) | `false` |

### Interactive Console Commands

When running without `-k`, the following commands are available:

| Command | Description |
|---------|-------------|
| `a` | Show invalid packet count |
| `c` | Show verified and total server counts |
| `l` | Show last client info |
| `r` | Reload configuration |
| `s` | Show server counts per master |
| `x` | Exit |

### Signals

| Signal | Action |
|--------|--------|
| `SIGHUP` | Reload configuration |
| `SIGINT` | Graceful shutdown |
| `SIGTERM` | Graceful shutdown |

## Configuration

The server uses an XML configuration file. A default config is written automatically on first run. See below for the available options.

### Master Config (`<master-config>`)

| Option | Description | Default |
|--------|-------------|---------|
| `listen-address` | UDP address to listen on | `:28000` |
| `local-address` | Local address for outbound queries | _(empty)_ |
| `name` | Master server name | `Tribes Master` |
| `message-of-the-day` | MOTD shown to clients | _(example)_ |
| `rate-limit` | Max packets per second per client (min 1) | `2` |
| `master-query-time-seconds` | How often to query peer masters (60–600) | `120` |
| `game-query-time-seconds` | How often to query game servers (min 10) | `30` |
| `game-expire-queries` | Missed queries before a server is expired (min 1) | `2` |
| `build-packets-every-seconds` | How often to rebuild server list packets (min 1) | `1` |
| `min-packet-size` | Minimum UDP packet size (0 = no padding) | `0` |
| `masters` | List of peer master servers to query | _(see default config)_ |

### Web Config (`<web-config>`)

| Option | Description | Default |
|--------|-------------|---------|
| `listen-address` | HTTP address to listen on (empty = disabled) | `127.0.0.1:8080` |
| `build-pages-every-seconds` | How often to rebuild API pages (min 1) | `1` |
| `add-server-api-key` | API key for `/api/v1/addserver` (empty = no auth) | _(empty)_ |
| `add-server-rate-limit` | Max addserver requests per second per IP (min 1) | `10` |

## Web API

All API endpoints support `GET` and `HEAD` methods. Responses include `Access-Control-Allow-Origin: *` for CORS.

| Endpoint | Description |
|----------|-------------|
| `/` | Web frontend (Tribes-styled server browser) |
| `/api/v1/servers.json` | Game servers list (JSON) |
| `/api/v1/servers.xml` | Game servers list (XML) |
| `/api/v1/masters.json` | Master servers list (JSON) |
| `/api/v1/masters.xml` | Master servers list (XML) |
| `/api/v1/stats.json` | Stats summary (JSON) |
| `/api/v1/stats.xml` | Stats summary (XML) |
| `/api/v1/addserver` | Add a game server (POST, requires `?address=host:port`) |

## Deployment

### systemd Service

Create `/etc/systemd/system/t1ms.service`:

```ini
[Unit]
Description=Tribes 1 Master Server
After=network.target

[Service]
Type=simple
User=t1ms
Group=t1ms
ExecStart=/opt/t1ms/t1ms -k -c /opt/t1ms/config.xml
WorkingDirectory=/opt/t1ms
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/t1ms
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```sh
# Create user and install directory
sudo useradd -r -s /usr/sbin/nologin t1ms
sudo mkdir -p /opt/t1ms
sudo cp t1ms /opt/t1ms/
sudo chown -R t1ms:t1ms /opt/t1ms

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable t1ms
sudo systemctl start t1ms

# Reload config without restart
sudo systemctl reload t1ms
```

### NGINX Reverse Proxy

Example NGINX configuration to proxy the web interface with TLS:

```nginx
server {
    listen 443 ssl http2;
    server_name tribes.example.com;

    ssl_certificate     /etc/letsencrypt/live/tribes.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tribes.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name tribes.example.com;
    return 301 https://$host$request_uri;
}
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
