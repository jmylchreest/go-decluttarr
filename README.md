# go-decluttarr

A Go port of [decluttarr](https://github.com/ManiMatter/decluttarr) - automatically clean up your *arr download queues.

## Features

- **Removal Jobs**: Automatically remove stalled, slow, failed, orphaned, and problematic downloads
- **Multi-client Support**: Works with Sonarr, Radarr, Lidarr, Readarr, Whisparr, Sportarr, and Bookshelf
- **Shelfarr**: Read-only request monitoring and mandatory download-category protection
- **Download Clients**: qBittorrent at runtime; SABnzbd and NZBGet API packages exist but are not yet wired into the executable
- **Strike System**: Configurable strike threshold before removal (prevents false positives)
- **Tracker-Aware**: Different handling for private vs public trackers
- **Protected Downloads**: Tag torrents in qBittorrent to prevent removal
- **Graceful Failures**: Continues running even if individual services are unavailable
- **Structured Logging**: JSON logging with configurable levels

## Installation

### Docker (Recommended)

```bash
docker pull ghcr.io/jmylchreest/go-decluttarr:latest
```

### From Source

```bash
go install github.com/jmylchreest/go-decluttarr/cmd/go-decluttarr@latest
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
go-decluttarr --config config.yaml

# Specify data directory for strike persistence
go-decluttarr --config config.yaml --data /data

# Check version
go-decluttarr --version
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
go-decluttarr --config config.yaml 2>&1 | humanlog

# Kubernetes
kubectl logs -f deploy/go-decluttarr | humanlog
```

## Docker Compose

```yaml
services:
  go-decluttarr:
    image: ghcr.io/jmylchreest/go-decluttarr:latest
    environment:
      - LOG_LEVEL=info
      - LOG_FORMAT=json
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./data:/data
    command: ["--config", "/config/config.yaml", "--data", "/data"]
```

## Kubernetes

See `deploy/k8s/deployment.yaml` for a Kubernetes deployment with Secrets and a PersistentVolumeClaim. Pin its example image to a tested release digest before production use.

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

### Search Jobs (not active in the executable)

Search implementations are incomplete and not registered by `main`. Enabling their configuration currently does not run them.

| Job | Description |
|-----|-------------|
| `search_missing` | Search for missing episodes/movies (respects `min_days_between_searches`) |
| `search_unmet_cutoff` | Search for items not meeting quality cutoff |

## Tracker Handling

go-decluttarr can handle private and public tracker torrents differently:

| Mode | Behavior |
|------|----------|
| `remove` | Normal removal from queue and download client |
| `skip` / `keep` | Leave the download alone |
| `obsolete_tag` | Tag the torrent in qBittorrent instead of removing |

Configure with `private_tracker_handling` and `public_tracker_handling` in the general config.

## Protected Downloads

To prevent specific torrents from being removed, add the configured `protected_tag` (default: "Keep") to the torrent in qBittorrent. Protected torrents are skipped by all removal jobs.

## Sportarr, Bookshelf and Shelfarr

Add entries under `instances` (see [config.example.yaml](config.example.yaml)):

```yaml
instances:
  sportarr:
    - name: sports
      url: http://sportarr:1867
      api_key: your-sportarr-api-key
  bookshelf:
    - name: books
      url: http://bookshelf:8787
      api_key: your-bookshelf-api-key
  shelfarr:
    - name: requests
      url: http://shelfarr:3000
      api_key: your-shelfarr-bearer-token
      protected_categories: [shelfarr]
```

Sportarr and Bookshelf participate in queue cleanup and orphan ownership checks.
Sportarr uses its v3 compatibility queue and native event monitoring endpoint;
IPTV/DVR recordings are outside cleanup scope. Bookshelf uses the v1 API and
author monitoring, matching Readarr semantics. Slow-download cleanup skips
entries without an `added` timestamp, including current Bookshelf queues.
The existing runtime limitation to qBittorrent cleanup still applies.

Shelfarr support is **read-only monitoring plus protection**, not request
deletion or retry. Each cycle reports status counts for up to 100 recent requests
visible to the token; this is not a complete ownership inventory. Its configured
`protected_categories` are required, case-sensitive, and protected on every
download client, including on API errors or an empty response. List every category
Shelfarr uses. Keep categories dedicated to one owner.

Start with `general.test_run: true` and inspect the proposed actions. Contract
tests use synthetic fixtures derived from upstream source; these integrations
have not yet been validated against your running instances.

## Shared download clients and Shelfarr

Keep `remove_orphans` disabled when sharing a download client with Shelfarr or other unsupported managers. Absence from an *arr queue does not prove that a download is unwanted.

Orphan cleanup now refuses to run without `target_categories`, or if any configured queue cannot be fetched completely. Only list categories used **exclusively** by the configured *arr instances:

```yaml
jobs:
  remove_orphans:
    enabled: false
    target_categories: ["tv-sonarr", "movies-radarr"]
```

Never include Shelfarr categories, shared categories, or uncategorized downloads. `remove_done_seeding` also requires explicit target categories or tags and honors protected tags and tracker policies. Unknown torrent ownership or tracker status now skips removal. Configured Shelfarr categories override cleanup scopes and remain protected even when its API is unavailable.

After upgrading, review accumulated strikes from the old orphan rule before enabling it again. The local source changes must be built and deployed to affect a running installation.

## Maintenance and research

See [the maintenance audit](docs/maintenance-audit.md) for security results, performance measurements, automation setup, compatibility changes, and remaining limitations. See [the integration research](docs/integrations.md) for Shelfarr, Audiobookshelf, Bookshelf, Listenarr, Sportarr, and the proposed roadmap.

## License

MIT
