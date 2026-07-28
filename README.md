# Pixelgrama

Pixelgrama is a public bilingual wall of 16×16 pixel-art postcards. A visitor draws with a fixed 16-color VGA palette, may sign with a short alias, and publishes the numeric postcard to a shared wall.

## Data contract

A postcard contains only:

- `pixels`: exactly 256 JSON integers, each from `0` through `15`.
- `alias`: optional, at most 16 ASCII characters matching `[A-Za-z0-9 _-]`.

The server rejects malformed JSON, unknown fields, non-arrays, wrong lengths, non-integers, values outside the palette, oversized bodies and invalid aliases with explicit JSON errors. It accepts no free text, uploaded image, HTML, SVG or arbitrary base64 data. SQLite stores the pixels as a checked 256-byte value, the optional alias, creation time and deployed commit.

## HTTP surface

The HTTP surface is intentionally small:

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/` | Permanent redirect to `/wall`. |
| `POST` | `/postcard` | Validate and publish one JSON postcard. |
| `GET` | `/wall` | Embedded HTML wall, or bounded JSON with `Accept: application/json` / `?format=json`. |
| `GET` | `/healthz` | Process liveness; does not depend on SQLite. |
| `GET` | `/readyz` | SQLite and schema readiness. |
| `GET` | `/version` | Deployed commit, repository and pull-request provenance. |

Wall pagination defaults to 24 entries, caps at 64 and caps the page number at 1000. Results are newest first. Pixel-identical consecutive visible submissions are rejected even when their aliases differ. Administratively hidden postcards are excluded from the wall and do not affect public deduplication.

## Architecture and security

- Go standard library plus `modernc.org/sqlite`; no CGO.
- One binary with HTML, CSS and JavaScript embedded through `go:embed`.
- Vanilla canvas frontend; no Node runtime, CDN, remote font, analytics or third-party browser dependency.
- Fixed-window POST rate limit by client IP with bounded memory and validated operational defaults. Forwarded headers are accepted only when the immediate peer belongs to `TRUSTED_PROXY_CIDRS`; otherwise they are ignored.
- CSP explicitly declares `default-src`, `connect-src`, `script-src`, `style-src`, `img-src`, `base-uri`, `form-action`, `frame-ancestors` and `object-src`. Inline embedded CSS and JavaScript are allowed only by exact SHA-256 hashes generated from the embedded bytes. `unsafe-inline` is never used.
- `nosniff`, no-referrer, restrictive Permissions Policy, same-origin opener/resource policies and no-store responses.
- SQLite WAL, busy timeout, one connection, transactional latest-postcard deduplication and schema migrations controlled by `PRAGMA user_version`. Databases newer than the binary are rejected.
- The container runs as UID/GID 10001, drops every capability, uses a read-only root filesystem in Compose and writes only to `/data`.

## Local development

```sh
go test ./...
CGO_ENABLED=0 go run ./cmd/pixelgrama
```

Override `ADDR` or `DB_PATH` through environment variables. Operational controls are `TRUSTED_PROXY_CIDRS`, `RATE_LIMIT_REQUESTS`, `RATE_LIMIT_WINDOW` and `RATE_LIMIT_MAX_ENTRIES`. Invalid CIDRs, durations or non-positive limits stop startup. The default database path is `/data/pixelgrama.db`.

Full local gates:

```sh
go mod verify
test -z "$(gofmt -l .)"
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath ./cmd/pixelgrama
git diff --check
```

## Container, CI and Coolify

`Dockerfile` is multi-stage: Go compiles a static CGO-disabled binary and the final Alpine image runs it as a non-root user. GitHub Actions performs tests, race detection, vet and static build on pull requests. Only a merge to `main` or a version tag publishes images to GHCR.

The image workflow injects the exact commit, repository URL and pull request associated with that commit, publishes both `latest` and `sha-<full-commit>` tags, and smoke-tests `/healthz`, `/readyz`, `/version`, a verified SQLite backup and the administrative hide/restore flow.

`docker-compose.yml` has no `build` section. Coolify therefore pulls the CI-built public `ghcr.io/charle-z/pixelgrama:latest` image instead of compiling on the CPU-limited VPS. Deploy only after the image workflow for the merged commit is available, persist the named `/data` volume, expose port 8080 and use `/healthz` as the health check. Credentials, when required by the registry or platform, belong only in Coolify.

## Administrative moderation

Moderation is available only through the existing binary; there is no administrative HTTP endpoint or web panel. Commands return JSON on stdout:

```sh
pixelgrama admin list --status hidden --limit 100
pixelgrama admin hide --id 42 --reason "policy review"
pixelgrama admin restore --id 42 --reason "review completed"
```

`list` accepts `hidden`, `visible` or `all` and returns no pixel payloads. Reasons are required for state changes, limited to 256 Unicode characters and may not contain control characters. Each hide or restore is transactional: the current state, timestamp and reason are updated together with a persistent event. Operational logs record only action, postcard ID, resulting state and timestamp; they do not print the administrative reason.

## SQLite operations

The current schema version is `2`. Databases at versions `0` or `1` are migrated sequentially and transactionally without recreating or deleting existing postcards. Version 2 adds moderation state and a persistent moderation event history; existing postcards become visible. A database with a version greater than the supported version is rejected before serving traffic.

`/healthz` remains a liveness endpoint. `/readyz` executes a real SQLite ping and verifies that `PRAGMA user_version` matches the binary.

Create a consistent administrative backup with the same binary:

```sh
pixelgrama backup --output /backups/pixelgrama-$(date +%Y%m%dT%H%M%SZ).db
```

The destination must not already exist and must be writable by UID 10001. The command uses SQLite `VACUUM INTO`, then opens the copy and verifies `PRAGMA integrity_check`, schema version and the `postcards` table. Mount the destination on storage separate from the live `/data` volume.

Restoration is deliberately manual:

1. Stop `pixelgrama-prod`.
2. Preserve the current database and its WAL/SHM files.
3. Place the verified backup at the configured `DB_PATH` with UID/GID 10001 ownership and mode 0750 on its directory.
4. Start the application.
5. Require `/readyz`, `/healthz` and `/version` to succeed before reopening writes.

## Provenance

Every postcard records the build commit active when it was created. `/version` and the page footer expose that commit and link to the public repository and the pull request that produced the image.

## Built through MCP Devbox

Pixelgrama was constructed end to end by an agent operating through [MCP Devbox](https://github.com/charle-z/mcp-devbox): isolated Edge workspace, test-first implementation, Git publication, pull-request gates, image publication and reviewed Coolify deployment. The process log below is intentionally honest rather than polished into a perfect run.

## Process evidence

Times are UTC on 2026-07-27.

| Stage | Interval | Duration | Evidence and corrections |
| --- | --- | ---: | --- |
| Repository bootstrap | before 03:59:36 | recorded by orchestrator | The new empty GitHub repository had no clonable `main`; two Edge continuations terminated before code work. A minimal README commit created `main`, then the project was prepared again. |
| Tested core | 03:59:36–04:06:28 | 6m52s | Core tests were written first and failed on missing symbols. Validation, fixed encoding, SQLite persistence, deduplication and bounded rate limiting then passed. The first commit attempt failed because Git identity was absent; it was retried with command-local agent identity without changing Git configuration. |
| Hardened API | 04:06:28–04:14:13 | 7m45s | API tests were written first and failed on missing symbols. The first implementation classified a non-array `pixels` value as 422 instead of 400; an explicit structural check corrected it and the full suite passed. |
| Embedded frontend | 04:14:13–04:24:50 | 10m37s | Frontend contract tests first failed because bilingual markup and CSP-authorized inline assets did not exist. The embedded VGA canvas UI and exact asset hashes made them pass. |
| Packaging and CI | 04:24:50–04:34:19 | 9m29s | `go mod verify`, formatting, `go test -race ./...`, `go vet ./...`, a CGO-disabled build, `git diff --check` and a live static-binary smoke test all passed. Docker validation was not reproducible on the Edge because `docker` is not installed; the host was not changed, and the pinned CI workflow owns the container build and smoke test. |

See [BACKLOG.md](BACKLOG.md) for ideas deliberately excluded from the first release.
