# Pixelgrama

Pixelgrama is a public bilingual wall of 16×16 pixel-art postcards. A visitor draws with a fixed 16-color VGA palette, may sign with a short alias, and publishes the numeric postcard to a shared wall.

## Data contract

A postcard contains only:

- `pixels`: exactly 256 JSON integers, each from `0` through `15`.
- `alias`: optional, at most 16 ASCII characters matching `[A-Za-z0-9 _-]`.

The server rejects malformed JSON, unknown fields, non-arrays, wrong lengths, non-integers, values outside the palette, oversized bodies and invalid aliases with explicit JSON errors. It accepts no free text, uploaded image, HTML, SVG or arbitrary base64 data. SQLite stores the pixels as a checked 256-byte value, the optional alias, creation time and deployed commit.

## HTTP surface

There are exactly four routes:

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/postcard` | Validate and publish one postcard. |
| `GET` | `/wall` | Embedded HTML wall, or bounded JSON with `Accept: application/json` / `?format=json`. |
| `GET` | `/healthz` | Liveness response. |
| `GET` | `/version` | Deployed commit, repository and pull-request provenance. |

Wall pagination defaults to 24 entries, caps at 64 and caps the page number at 1000. Results are newest first. Pixel-identical consecutive submissions are rejected even when their aliases differ.

## Architecture and security

- Go standard library plus `modernc.org/sqlite`; no CGO.
- One binary with HTML, CSS and JavaScript embedded through `go:embed`.
- Vanilla canvas frontend; no Node runtime, CDN, remote font, analytics or third-party browser dependency.
- Fixed-window POST rate limit by client IP with bounded memory. Proxy headers are ignored unless `TRUST_PROXY=true`; the supplied Coolify composition enables it behind the platform proxy.
- CSP explicitly declares `default-src`, `connect-src`, `script-src`, `style-src`, `img-src`, `base-uri`, `form-action`, `frame-ancestors` and `object-src`. Inline embedded CSS and JavaScript are allowed only by exact SHA-256 hashes generated from the embedded bytes. `unsafe-inline` is never used.
- `nosniff`, no-referrer, restrictive Permissions Policy, same-origin opener/resource policies and no-store responses.
- SQLite WAL, busy timeout, one connection and transactional latest-postcard deduplication.
- The container runs as UID/GID 10001, drops every capability, uses a read-only root filesystem in Compose and writes only to `/data`.

## Local development

```sh
go test ./...
CGO_ENABLED=0 go run ./cmd/pixelgrama
```

Override `ADDR`, `DB_PATH` or `TRUST_PROXY` through environment variables. The default database path is `/data/pixelgrama.db`.

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

The image workflow injects the exact commit, repository URL and pull request associated with that commit, publishes both `latest` and `sha-<full-commit>` tags, and smoke-tests `/healthz` plus `/version`.

`docker-compose.yml` has no `build` section. Coolify therefore pulls the CI-built image instead of compiling on the CPU-limited VPS. Production should set `PIXELGRAMA_IMAGE=ghcr.io/charle-z/pixelgrama:sha-<merge-commit>`, persist the named `/data` volume, expose port 8080 and use `/healthz` as the health check. Credentials, when required by the registry or platform, belong only in Coolify.

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
