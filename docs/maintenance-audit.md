# Maintenance audit

Reviewed 11 October 2026. The changes address unsafe cleanup decisions, update Go and dependencies, reduce queue latency, and add repeatable maintenance checks. They are local source changes: no release, deployment, live torrent deletion or remote repository setting change was performed.

## Dependencies and supply chain

Go is aligned at 1.27.1 in `go.mod` and Docker’s builder. Direct and indirect dependencies were updated using `go get -u -t ./...`, `go get -u all` and `go mod tidy`. Highlights include slog-logfilter 0.2.1, testify 1.12.1, mapstructure 2.5.0, go-toml 2.4.3, fsnotify 1.10.1, x/sys 0.49.0 and x/text 0.43.0. Viper remains 1.21.0, its current resolved release. Dependency updates are constrained to existing module paths; no unreviewed major-version migrations were introduced. The application and its tests use current module versions. `go list -m -u all` still shows newer versions for a few upstream development-only graph entries (go-cmp, go-internal, x/mod and x/tools); these are not in this project’s compiled dependency set, and `tidy` removes unused overrides.

GitHub checkout/setup-go and Docker actions were updated to current release commits and pinned by SHA. CI security tools are version-pinned. `.dockerignore` excludes local configuration, credentials, state and git metadata from the build context. The runtime remains a non-root scratch image.

## Security findings and fixes

| Finding | Change |
|---|---|
| Shelfarr and other downloads treated as orphans | Explicit exclusive category scope; no cleanup on missing or incomplete queue evidence |
| Only the first 1,000 *arr queue entries fetched | Pagination through the full queue; incomplete/repeated pages return errors |
| Default private-tracker `keep` fell through to removal | `keep` aliases `skip`; unknown policy, ownership or tracker status skips removal |
| Done-seeding job ignored protected tags and tracker policy | Same policy checks as other removal jobs; explicit category/tag scope required |
| Configured ignored download clients were not consistently honored | Ignore checks at queue-job item processing and direct-client enumeration; full ownership queues retained |
| SABnzbd API key logged in URLs and transport errors | Removed URL debug fields; transport errors omit request URLs |
| Redirects could forward custom API credentials | API redirects rejected; configure final service URLs |
| Unbounded HTTP error bodies | Error responses capped at 4 KiB in shared HTTP and *arr clients |
| SABnzbd mutation failures reported as success | Validate HTTP status and affirmative API status |
| Missing monitored flag interpreted as unmonitored | Treat absent/null flags as errors and skip destructive action |
| Strike files used predictable temp names and broad permissions | Unique temporary files, 0600 state files, 0750 created directories, serialized saves |
| Invalid/null persisted strike records could corrupt state | Validate a temporary map before replacing live records |
| Cancellation waited for the current cycle | Signal-derived context cancels in-flight API work; manager stops starting jobs |
| Kubernetes example used invalid `pause` policy and unscoped orphan cleanup | Uses `keep`; orphan cleanup disabled; configuration stored as a Secret; service-account token disabled; volume group set |

Gosec findings intentionally suppressed in place, with reasons: operator-selected local config paths (G304/G703), explicit TLS-verification opt-out (G402), and an outbound qBittorrent session cookie incorrectly assessed as a browser response cookie (G124). TLS verification stays enabled by default and minimum TLS is 1.2. No scanner rule is globally disabled.

## Verification and limits

- Full `go test -race ./...`, `go vet ./...`, and `go mod verify`: pass.
- Go vulnerability scanner 1.9.0: no known reachable vulnerabilities in the updated source build.
- Gosec 2.29.0: no unsuppressed findings after the fixes above.
- Gitleaks 8.30.1: no detected secrets in 12 local git commits. The working-tree scan identifies four credentials in ignored `config.yaml`; these are local runtime inputs, not committed findings. Their values were not included in the report. Do not add that file to source control.
- Trivy 0.75.0: zero Go-module vulnerabilities. Deployment findings are triaged below; this is not a claim that a running cluster was audited.
- Linux amd64 and arm64 static binaries build; amd64 `--version` smoke test and binary-mode govulncheck pass.
- Actionlint and the official Renovate configuration validator pass.
- Meaningful regression cases cover Shelfarr category exclusion, hash normalization, incomplete queues, dry-run, private/protected downloads, unknown tracker state, bounded concurrency, pagination, redirects, secret-bearing transport URLs, malformed monitored responses, and concurrent persistence.

The container build cannot run on this host: Podman/Buildah fails when mounting an overlay over the build context, including with temporary VFS storage. Consequently the final container image has not been locally smoke-tested or scanned. CI now builds a local test image, smoke-tests it, and scans it before the publishing job can proceed.

Residual deployment-template findings: the example uses `:latest` rather than a production digest, trusted-registry policy must be configured for the target cluster, and the scratch daemon has no meaningful HEALTHCHECK yet. Pin the deployed image to the tested release digest. A health check that merely runs `--version` would not detect a stuck cycle and was deliberately not added. ConfigMap secret warnings were resolved by storing configuration in a Kubernetes Secret.

Test coverage remains uneven: removal jobs have about 15% statement coverage overall and search code has no tests. The new tests protect the changed destructive paths; they do not establish production compatibility with every server version. Subsequent read-only checks against Sportarr 4.1.9.1119 verified authenticated system status and an empty v3 queue. Populated queues, mutations, Shelfarr and Bookshelf remain unverified against live instances.

## Performance measurements

`BenchmarkGetAllQueues` uses eight independent clients against a local HTTP server with 5 ms latency per queue. Each sample fetches eight one-item queues. Go 1.27.1, Linux amd64, Ryzen 7 9800X3D; three runs with `-benchmem`, without race instrumentation.

| Metric | Before | After |
|---|---:|---:|
| Median queue fetch | 41.704 ms | 10.487 ms |
| Median allocated bytes | 74,696 | 75,435 |
| Median allocations | 772 | 765 |
| Simultaneous queue requests | 1 | At most 4 |

This is a 74.9% latency reduction for the synthetic multi-instance workload, with roughly 1% more allocated bytes. It is not an end-to-end production latency guarantee. The number of queue requests is unchanged; they overlap across instances. A single-instance installation will not see this speedup.

A follow-up CPU profile collected 260 ms of CPU samples during 3.46 seconds of wall time, consistent with this workload spending most time waiting on I/O. There was no evidence justifying low-level JSON or allocator rewrites.

Two further I/O reductions have deterministic correctness tests or direct call-path evidence:

- Unmonitored checks reuse a successful parent-entity lookup within one instance/job run. Ten episodes sharing one series require one successful lookup instead of ten. Errors are not cached; the next run fetches again.
- Orphan/done-seeding jobs pass the already-fetched torrent and owning client into protection checks, avoiding another search across download clients.

Jobs remain sequential and queue results are fetched anew per job. A global cycle cache would become stale after deletion and could remove the same item twice; defer it until explicit invalidation exists. Strike persistence still writes the full small state file once per cycle; new locks and snapshots fix correctness without introducing a database. Benchmark very large queues and real API latency before expanding concurrency beyond four or changing storage.

Reproduce:

```sh
go test ./internal/jobs -run='^$' -bench=BenchmarkGetAllQueues -benchmem -count=3
go test ./internal/jobs -run='^$' -bench=BenchmarkGetAllQueues -cpuprofile=/tmp/queue-cpu.prof -memprofile=/tmp/queue-mem.prof -o /tmp/queue.test
go tool pprof -top /tmp/queue-cpu.prof
```

## Autonomous maintenance

`quality.yaml` runs through the build workflow on pull requests and pushes, every Monday at 06:23 UTC, and manually. It checks tests/races, vet, dependencies, workflow syntax, source security, secrets, deployment configuration and a built image. Image publication depends on this workflow succeeding. Failed scheduled runs use normal GitHub Actions notifications.

`renovate.json` prepares dependency PRs for Go modules, Docker, GitHub Actions and pinned scanner versions. Stable Go patch updates may merge after all reported checks pass and a three-day release-age delay. Go compiler updates, pre-1.0 libraries, minor/major changes, workflow changes and container updates require review. [Renovate automerge behavior](https://docs.renovatebot.com/key-concepts/automerge/)

Activation requires connecting the Renovate GitHub App or an existing self-hosted Renovate runner to this repository and merging these files. Protect the default branch and require the reusable workflow’s Verify, Security and Image checks plus Build and Push. The app connection, branch rules and GitHub scheduled execution were not changed or exercised locally. Do not bypass required checks or enable unconditional bot merging.

## Upgrade notes

Keep `remove_orphans` disabled for the current Shelfarr setup until exclusive categories exist and the new build is deployed. Review old orphan strikes before re-enabling. Done-seeding jobs without an explicit scope now report an error and skip; private `keep` now actually preserves torrents. `pause` was never implemented and is rejected; use `keep`/`skip`, `remove`, or `obsolete_tag`. Unverifiable torrent ownership now skips removal, including when no compatible download client is registered. Final API URLs are required because redirects are blocked.

See [integration research](integrations.md) for the feature backlog, including unsupported runtime options, ownership-aware strike keys, book integrations and search completion.


## Integration implementation follow-up

Sportarr and Bookshelf are now registered queue owners. Sportarr uses its v3
compatibility queue and native event monitoring; Bookshelf uses v1 and author
monitoring. Shelfarr has a separate read-only request monitor with mandatory
category protection that survives API outages. No live deployments or DNS
changes were made.

Additional tests cover executable registration, upstream-derived queue fixtures,
explicit false deletion flags, decimal queue sizes, missing download timestamps,
event/author identity mappings, Shelfarr authentication and response validation,
the 100-request cap, and protection despite overlapping cleanup scopes and an
API outage. Full race-enabled tests, go vet, executable build, gosec and
govulncheck passed after these changes. Sportarr status and empty-queue reads subsequently passed against 4.1.9.1119; populated queues and other live integrations remain pending.
See [integration setup and boundaries](../README.md#sportarr-bookshelf-and-shelfarr).
