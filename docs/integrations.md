# Integration research and roadmap

Reviewed 11 October 2026. The first priority is protecting downloads owned by other applications. Shelfarr request monitoring and category protection are now implemented, but the current public API does not supply the identifiers needed to make destructive ownership decisions safely. Audiobookshelf should be treated as a library destination, with any future integration limited initially to read-only status.

## Why the 19 Shelfarr downloads were selected

The previous orphan job enumerated every download in the client and selected anything absent from the configured Sonarr/Radarr/Lidarr/Readarr/Whisparr queues. Shelfarr downloads are absent by design. The rule also continued after queue failures and only fetched the first 1,000 records, creating two more routes to false orphan decisions.

This change requires explicit, nonempty categories dedicated exclusively to configured *arr instances, fetches every queue page, compares download IDs without case sensitivity, and stops on incomplete queue evidence. Unscoped done-seeding cleanup also stops. Protected tags and private-tracker policies apply to both direct-client jobs. A shared category still cannot prove ownership: keep orphan cleanup disabled in that situation.

Shelfarr now has a separate read-only request adapter and mandatory protected categories. It is not registered as an *arr ownership queue. These safeguards do not prove that all downloads absent from a queue are unwanted. In particular, completed imports can leave the queue while remaining in the download client for seeding; assign categories and policies accordingly.

## Candidate integrations

| Application | Role and upstream evidence | Fit for go-decluttarr | Priority |
|---|---|---|---|
| Shelfarr, Pedro-Revez-Silva | Requests, acquisition, processing and delivery to library platforms; distinct REST API | Implemented: read-only request counts and category protection; destructive ownership requires upstream identifiers | High |
| Audiobookshelf | Libraries, items, scans and playback API | Library health and optional scan notifications; not torrent ownership authority | Medium |
| Bookshelf | Readarr revival with working metadata sources | Implemented: v1 queue/delete and author monitoring, covered by contract tests | High |
| Listenarr | Audiobook acquisition and library organization | Separate adapter, verify its actual API rather than assuming Radarr compatibility | Medium |
| Sportarr | Sports PVR; upstream decluttarr already lists support | Implemented: v3 queue/delete and native event monitoring; no DVR cleanup or search registration | Medium |
| Shelfmark | Book/audiobook search and requests across several source types | Protect its categories now; consider ownership reporting later | Medium |
| Prowlarr | Indexer manager/proxy | Optional health diagnostics; not an authoritative download queue | Low |
| Seerr | Media requests and discovery for Jellyfin/Plex/Emby | Avoid duplicating requests; possible event/status integration only | Low |

Sources: [Shelfarr](https://github.com/Pedro-Revez-Silva/shelfarr), [Audiobookshelf API](https://api.audiobookshelf.org/), [Bookshelf](https://github.com/pennydreadful/bookshelf), [Listenarr](https://github.com/Listenarrs/Listenarr), [Sportarr](https://github.com/Sportarr/Sportarr), [upstream decluttarr](https://github.com/ManiMatter/decluttarr), [Shelfmark](https://github.com/calibrain/shelfmark), [Prowlarr](https://github.com/Prowlarr/Prowlarr), [Seerr](https://github.com/seerr-team/seerr).

### Shelfarr contract and implementation prerequisites

Shelfarr exposes `/api/v1/requests` with bearer tokens and request states including downloading, processing, completed and failed. Ordinary user tokens only see their own requests; admin/global tokens can see all. A partial user view therefore cannot establish that an unlisted download is an orphan. The documented list accepts a limit; completeness must be verified explicitly. [API documentation](https://raw.githubusercontent.com/Pedro-Revez-Silva/shelfarr/main/docs/api.md)

The inspected request serializer returns request, book and user metadata but no torrent hash, download-client identifier or NZB ID. Selected-result data likewise lacks these ownership identifiers. Consequently, request/title matching is insufficient for deletion. [Controller implementation](https://raw.githubusercontent.com/Pedro-Revez-Silva/shelfarr/main/app/controllers/api/v1/requests_controller.rb)

Proposed adapter acceptance criteria:

1. A stable, versioned API exposes all active and importing downloads, with client identity, download ID, category, lifecycle state and explicit pagination/completeness semantics.
2. Token visibility is verified across users. Permission failures, timeouts, unknown states and incomplete pages make ownership unknown, never absent.
3. Read-only discovery ships first. Deletion/retry remains disabled until fixture and real-instance tests cover processing, imported-but-seeding, duplicate requests and client outages.
4. Cancellation, blocklisting and download deletion remain distinct operations. A request cancellation must not silently become file deletion.

The API has retry and blocklist-and-next operations, but these should stay under Shelfarr’s control until a clear responsibility boundary is established. Avoid two services independently retrying the same request.

Another project was advertised as `VacantlyCrushing/Shelfarr`, for file normalization. Its repository could not be verified during this review; it is not assumed equivalent to the acquisition application above. Confirm repository identity when discussing “Shelfarr support.”

### Audiobookshelf boundary

Audiobookshelf provides library/item and scan APIs with bearer authentication. These are useful for confirming library availability after import, but library presence does not by itself associate an item with a particular torrent. Do not infer ownership from title or filesystem names. Shelfarr already offers library scan integration, so a second automatic scan source needs deduplication and a specific unmet need. [Audiobookshelf API](https://api.audiobookshelf.org/), [Shelfarr integrations](https://github.com/Pedro-Revez-Silva/shelfarr)

A useful initial feature would be read-only library connectivity/status in cycle diagnostics. Import confirmation could follow only with reliable correlation identifiers and permissions limited to the necessary libraries. Missing library items must never trigger torrent deletion automatically.

### Readarr and compatibility

Readarr officially retired after its metadata service became unusable and replacement work stalled. Retain its existing adapter for users who still run it, but label it legacy and stop treating it as the primary future audiobook path. [Readarr announcement](https://raw.githubusercontent.com/Readarr/Readarr/develop/README.md)

Bookshelf describes itself as a Readarr revival with Goodreads/Hardcover metadata support. The first-class `instances.bookshelf` adapter now uses its v1 queue and delete endpoints and author monitored status. Contract tests cover decimal queue sizes and missing added timestamps; slow cleanup skips records without that timestamp. Search jobs remain unregistered. [Bookshelf](https://github.com/pennydreadful/bookshelf)

## Existing features to update or remove

The repository advertised more runtime support than it actually registered. The README now states that SABnzbd/NZBGet packages and search implementations are not wired into the executable. Merely adding YAML keys does not activate these features.

Before expanding integrations:

- Add startup capability validation so unsupported jobs and per-instance filters cannot silently do nothing. Audit `enabled`, enabled/disabled job lists, protected/ignored/only tags and per-job override fields before advertising them.
- Finish SABnzbd/NZBGet runtime adapters with Usenet-specific semantics; do not pretend every item has torrent properties or private trackers.
- Finish search registration only after durable search cooldowns, cancellation and bounded concurrency are verified. The missing-search code can otherwise block with zero concurrency; cutoff search is incomplete.
- Namespace strikes by owning client, download and rule. The current download-ID-only key shares strikes across rules and clients, which is a poor foundation for additional owners.
- Add per-cycle maximum removals, a minimum orphan age and a dry-run action report before broadening cleanup scope. Preserve the existing default-disabled jobs.
- Add metrics for API latency, scanned items, skipped/unknown ownership, planned actions and successful actions. A cycle heartbeat would also permit a meaningful container health check.
- Evaluate qBittorrent API-key authentication for newer servers alongside session-cookie login; upstream decluttarr now documents that option. Preserve backwards compatibility with existing installations. [Upstream configuration](https://github.com/ManiMatter/decluttarr)

Recommended order: validate these safety fixes and new integrations in dry-run against real instances; improve ownership/capability contracts; then consider Listenarr and additional read-only diagnostics. Keep media requests, indexer management and library playback outside this tool’s core responsibility.


## Implemented contract boundaries

Source-derived synthetic fixtures and local HTTP contract tests cover Sportarr
v3 queues, native system/event endpoints, Bookshelf v1 queues and author status,
explicit delete flags, and Shelfarr bearer-authenticated request lists. They are
not captured production responses or a substitute for real-instance dry runs.

Sportarr delete defaults `removeFromClient` to true upstream, so every boolean
delete option is sent explicitly, including false. Its compatibility API maps
event IDs to `episodeId`; pending imports without an event ID are skipped by
unmonitored cleanup. IPTV live recording is upstream alpha functionality and
is outside this adapter's scope.

Shelfarr exposes at most 100 recent requests with no pagination or download
identity. Counts are token-scoped diagnostics only. Mandatory category protection
is local configuration and remains active on authentication failures, outages,
unknown request states and empty responses. No request mutation is implemented.

Reviewed source revisions:
- [Sportarr 9d3ef23](https://github.com/Sportarr/Sportarr/tree/9d3ef23ec2d8dcf76187a43a1881977eb12a1d7e): `src/Endpoints/SonarrQueueEndpoints.cs`, `EventEndpoints.cs`.
- [Bookshelf 3ac41f7](https://github.com/pennydreadful/bookshelf/tree/3ac41f7e31427684a37138e8e539b7e6c7b73869): `src/Readarr.Api.V1/Queue/QueueResource.cs`, `QueueController.cs`.
- [Shelfarr 015fa71](https://github.com/Pedro-Revez-Silva/shelfarr/tree/015fa71d06203b9c189f1eb48e156d61a61710dc): `app/controllers/api/v1/requests_controller.rb`, `application_controller.rb`.
