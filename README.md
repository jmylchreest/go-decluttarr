# go-declutarr

A Go port of [decluttarr](https://github.com/ManiMatter/decluttarr) - automatically clean up your *arr download queues.

## Features

- **Removal Jobs**: Automatically remove stalled, slow, failed, orphaned, and problematic downloads
- **Search Jobs**: Trigger searches for missing items and quality upgrades
- **Multi-client Support**: Works with Sonarr, Radarr, Lidarr, Readarr, and Whisparr
- **Download Clients**: qBittorrent, SABnzbd, and NZBGet
- **Strike System**: Configurable strike threshold before removal (prevents false positives)
- **Tracker-Aware**: Different handling for private vs public trackers
- **Protected Downloads**: Tag torrents in qBittorrent to prevent removal
- **Graceful Failures**: Continues running even if individual services are unavailable
- **Structured Logging**: JSON logging with configurable levels

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
  test_run: false                      # Set true to log without removing
  timer: 10m                           # How often to run
  ssl_verification: true
  request_timeout: 30s
  private_tracker_handling: keep       # remove, skip, or obsolete_tag
  public_tracker_handling: remove      # remove, skip, or obsolete_tag
  protected_tag: "Keep"                # qBit tag that prevents removal
  obsolete_tag: "Obsolete"             # Tag applied when using obsolete_tag mode
  ignore_download_clients: []          # Client names to skip

jobs:
  remove_stalled:
    enabled: true
    max_strikes: 3
  remove_slow:
    enabled: true
    min_download_speed: 100            # KB/s
  remove_failed_imports:
    enabled: true
    message_patterns:                  # Custom patterns (optional)
      - "*Not an upgrade*"
      - "*Sample*"
  remove_bad_files:
    enabled: true
    keep_archives: false               # Set true to preserve .zip/.rar files
  remove_done_seeding:
    enabled: true
    target_tags: ["completed"]         # Filter by qBit tags
    target_categories: ["tv-sonarr"]   # Filter by categories
  search_missing:
    enabled: true
    min_days_between_searches: 7
    max_concurrent_searches: 3

instances:
  sonarr:
    - name: sonarr
      url: http://sonarr:8989
      api_key: your-api-key
  radarr:
    - name: radarr
      url: http://radarr:7878
      api_key: your-api-key
  whisparr:                            # Whisparr support
    - name: whisparr
      url: http://whisparr:6969
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

# Check version
go-declutarr --version
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
| `remove_failed_imports` | Remove downloads that failed to import (supports custom `message_patterns`) |
| `remove_orphans` | Remove downloads not tracked by any *arr instance |
| `remove_missing_files` | Remove queue items where files no longer exist |
| `remove_unmonitored` | Remove downloads for unmonitored content |
| `remove_bad_files` | Remove downloads with problematic files (supports `keep_archives`) |
| `remove_metadata_failed` | Remove downloads with metadata extraction failures |
| `remove_done_seeding` | Remove completed torrents that met seeding goals |

### Search Jobs

| Job | Description |
|-----|-------------|
| `search_missing` | Search for missing episodes/movies (respects `min_days_between_searches`) |
| `search_unmet_cutoff` | Search for items not meeting quality cutoff |

## Tracker Handling

go-declutarr can handle private and public tracker torrents differently:

| Mode | Behavior |
|------|----------|
| `remove` | Normal removal from queue and download client |
| `skip` | Leave the download alone |
| `obsolete_tag` | Tag the torrent in qBittorrent instead of removing |

Configure with `private_tracker_handling` and `public_tracker_handling` in the general config.

## Protected Downloads

To prevent specific torrents from being removed, add the configured `protected_tag` (default: "Keep") to the torrent in qBittorrent. Protected torrents are skipped by all removal jobs.

## License

MIT
