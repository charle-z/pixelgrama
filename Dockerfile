FROM golang:1.26.5-alpine3.23 AS build

ARG COMMIT=development
ARG REPO_URL=https://github.com/charle-z/pixelgrama
ARG PR_URL=https://github.com/charle-z/pixelgrama/pulls

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X main.Commit=${COMMIT} -X main.RepoURL=${REPO_URL} -X main.PRURL=${PR_URL}" \
    -o /out/pixelgrama ./cmd/pixelgrama

FROM alpine:3.23

RUN addgroup -S -g 10001 pixelgrama \
    && adduser -S -D -H -u 10001 -G pixelgrama pixelgrama \
    && mkdir -p /data \
    && chown 10001:10001 /data

COPY --from=build /out/pixelgrama /usr/local/bin/pixelgrama

USER 10001:10001
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/pixelgrama"]
