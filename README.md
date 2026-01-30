# go-declutarr

A Go port of [decluttarr](https://github.com/ManiMatter/decluttarr) - automatically clean up your *arr download queues.

## Features

- **Removal Jobs**: Automatically remove stalled, slow, failed, orphaned, and problematic downloads
- **Search Jobs**: Trigger searches for missing items and quality upgrades
- **Multi-client Support**: Works with Sonarr, Radarr, Lidarr, and Readarr
- **Download Clients**: qBittorrent, SABnzbd, and NZBGet
- **Strike System**: Configurable strike threshold before removal (prevents false positives)
- **Graceful Failures**: Continues running even if individual services are unavailable
- **Structured Logging**: slog-based logging with JSON output support

## Installation

### Docker (Recommended)

```bash
docker pull ghcr.io/jmylchreest/go-declutarr:latest
```

### From Source

```bash
go install github.com/jmylchreest/go-declutarr/cmd/go-declutarr@latest
```

## Configuration

Copy `config.example.yaml` to `config.yaml` and edit with your settings:

```yaml
general:
  log_level: info
  test_run: false      # Set true to log without removing
  timer: 10m           # How often to run

jobs:
  remove_stalled:
    enabled: true
    max_strikes: 3
  remove_orphans:
    enabled: true
  # ... see config.example.yaml for all options

instances:
  sonarr:
    - name: sonarr
      url: http://sonarr:8989
      api_key: your-api-key

download_clients:
  qbittorrent:
    - name: qbittorrent
      url: http://qbittorrent:8080
      username: admin
      password: your-password
```

## Usage

```bash
# Run with config file
go-declutarr --config config.yaml

# Specify data directory for strike persistence
go-declutarr --config config.yaml --data /data
```

## Logging

Logs are output in JSON format by default (recommended for log aggregators). Environment variables override config:

| Variable | Values | Default |
|----------|--------|---------|
| `LOG_LEVEL` | debug, info, warn, error | info |
| `LOG_FORMAT` | json, text | json |

For pretty output locally, pipe through [humanlog](https://github.com/humanlogio/humanlog):

```bash
# Local development
go-declutarr --config config.yaml 2>&1 | humanlog

# Kubernetes
kubectl logs -f deploy/go-declutarr | humanlog
```

## Docker Compose

```yaml
services:
  go-declutarr:
    image: ghcr.io/jmylchreest/go-declutarr:latest
    environment:
      - LOG_LEVEL=info
      - LOG_FORMAT=json
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./data:/data
    command: ["--config", "/config/config.yaml", "--data", "/data"]
```

## Kubernetes

See `deploy/k8s/deployment.yaml` for a complete Kubernetes deployment with ConfigMap, Secret, and PersistentVolumeClaim.

## Jobs

### Removal Jobs

| Job | Description |
|-----|-------------|
| `remove_stalled` | Remove downloads stuck in stalled state |
| `remove_slow` | Remove downloads below minimum speed threshold |
| `remove_failed_downloads` | Remove downloads that failed to complete |
| `remove_failed_imports` | Remove downloads that failed to import |
| `remove_orphans` | Remove downloads not tracked by any *arr instance |
| `remove_missing_files` | Remove queue items where files no longer exist |
| `remove_unmonitored` | Remove downloads for unmonitored content |
| `remove_bad_files` | Remove downloads with problematic file issues |
| `remove_metadata_failed` | Remove downloads with metadata extraction failures |

### Search Jobs

| Job | Description |
|-----|-------------|
| `search_missing` | Search for missing episodes/movies |
| `search_unmet_cutoff` | Search for items not meeting quality cutoff |

## License

MIT
