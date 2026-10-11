# v0.2.0 release notes and checklist

Status: prepared on release/v0.2.0 in draft PR #3; not tagged or published.

## Release notes

This release adds Sportarr and Bookshelf queue support, Shelfarr request
monitoring and download-category protection, and qBittorrent 5.2 session-cookie
compatibility. It fixes unsafe orphan selection and private-tracker handling,
hardens HTTP credentials and persisted state, and updates Go and dependencies.

Queue fetching now runs across up to four instances concurrently. An eight-client
synthetic benchmark improved from 41.704 ms to 10.487 ms (74.9% less latency);
allocated bytes increased about 1%, so this is primarily an I/O improvement.

Shelfarr monitoring is read-only and limited to recent requests visible to the
token. It does not establish torrent ownership or perform retries/deletions.
Sportarr IPTV/DVR recordings are outside cleanup scope. Bookshelf slow-download
checks skip entries without a start timestamp. SABnzbd/NZBGet and search jobs
remain unwired at runtime.

### Upgrade requirements

- Keep orphan cleanup disabled until dedicated categories are reviewed.
  Enabled orphan cleanup requires explicit exclusive target_categories and
  complete ownership queues. Review pre-upgrade orphan strikes before enabling.
- Done-seeding cleanup requires explicit category or tag scope and now honors
  protected tags and tracker policy. Private tracker keep now preserves downloads.
- Replace unsupported pause policy with keep/skip, remove or obsolete_tag.
- Shelfarr configuration requires every download category in protected_categories.
- Use final API URLs: redirects are rejected to protect credentials.
- Start the new configuration with general.test_run: true and review actions.
- Preserve configuration and strike data; account for non-root UID/GID 65534
  when mounting state volumes.

### Validation

Race-enabled tests, vet, Go module verification, source vulnerability/security
checks and static builds pass locally. Sportarr 4.1.9.1119 status and empty queue
reads passed live. Populated queues and live Bookshelf/Shelfarr remain unverified.
The local container build is blocked by the host's Podman mount failure. CI's
container build, smoke test and image scan passed after patching Go to 1.27.2.
All checks must pass on the final release commit before publication.

## Publishing checklist

1. Push the reviewed commits to a branch and open a PR against main.
2. Require the Build and Push workflow to pass, including Quality Verify,
   Security and Image jobs and both target architecture builds.
3. Merge and wait for main CI to pass at the exact release commit.
4. Create an annotated v0.2.0 tag on that commit and push the tag. This triggers
   publication to ghcr.io/jmylchreest/go-decluttarr with 0.2.0, 0.2, 0 and latest
   tags for linux/amd64 and linux/arm64. Treat updating latest as a rollout event
   for any installations that automatically track it.
5. Wait for the tag workflow to pass; verify image version/commit and both
   architecture manifests. Record the published digest.
6. Create a GitHub release using the release-notes section above. The existing
   workflow publishes containers but does not create a GitHub Release or attach
   standalone binaries.
7. Roll out the tested digest separately, initially in dry-run mode. Retain the
   previous image digest and configuration backup for rollback.

Version is injected from the tag; no source version constant needs changing.
Build date comes from the commit timestamp, including tag-triggered builds.
Renovate and scheduled maintenance become active only once merged and the
Renovate app/runner is connected; branch-required checks need repository setup.
