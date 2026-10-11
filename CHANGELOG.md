# Changelog

## 0.2.1 — 2026-10-11

- Update slog-logfilter from v0.2.1 to v0.3.0 and migrate runtime controls to
  its handler-scoped API. Application level and job-filter changes now reach
  existing child loggers without affecting independently created loggers.
- Add regression coverage for JSON/text output, default logger installation,
  dynamic levels, child filters and logger-family isolation.
- Log sanitization remains disabled; this update preserves the existing output
  policy and does not enable upstream optional redaction rules.

## 0.2.0 — 2026-10-11

### Added

- Sportarr queue cleanup and event monitoring; IPTV/DVR recordings are excluded.
- Bookshelf queue cleanup and author monitoring through its Readarr-compatible API.
- Shelfarr read-only request monitoring and mandatory protected download categories.
- Opt-in `remove_metadata_downloading` job for torrents stuck fetching swarm
  metadata, with configurable message patterns and independent strikes.
  Resolves [#2](https://github.com/jmylchreest/go-decluttarr/issues/2).
- Scheduled quality/security checks and Renovate dependency maintenance configuration.

### Fixed

- Orphan cleanup now requires explicit categories and complete, paginated queue evidence.
- Private-tracker `keep`, protected tags and ignored-client settings are honored
  consistently, including direct-client cleanup jobs.
- qBittorrent 5.2 session cookies and successful empty responses are supported.
- Missing monitored flags and download timestamps no longer produce unsafe cleanup decisions.
- HTTP redirects cannot forward API credentials; sensitive request URLs are
  omitted from transport errors and shared HTTP/*arr error-response bodies are bounded.
- Strike persistence uses restrictive permissions, unique temporary files and
  serialized writes; invalid persisted records are rejected.
- Go updated to 1.27.2 with dependencies and CI actions refreshed.

### Performance

- Queue reads overlap across up to four instances. An eight-client synthetic
  benchmark improved from 41.704 ms to 10.487 ms; allocated bytes increased ~1%.
- Reuse parent monitoring lookups within each job and already-fetched torrents
  in direct-client protection checks.

### Upgrade notes

- Keep orphan cleanup disabled until dedicated `target_categories` and old
  orphan strikes are reviewed. Never include shared or Shelfarr categories.
- Done-seeding cleanup requires explicit category or tag scope.
- Replace unsupported `pause` tracker policy with `keep`/`skip`, `remove` or
  `obsolete_tag`. Private tracker `keep` now preserves downloads.
- Shelfarr requires `protected_categories`; monitoring is limited to recent
  requests visible to the token and never authorizes deletion or retries.
- `remove_metadata_failed` still means library-matching failures. Use the new,
  disabled-by-default `remove_metadata_downloading` job for swarm metadata.
- Use final API URLs because redirects are rejected.
- Preserve configuration and strike data, ensure state volumes are writable by
  the container user (65534:65534), and start with `general.test_run: true`.
- Runtime cleanup remains limited to qBittorrent; SABnzbd/NZBGet and search jobs
  are not wired into the executable.
