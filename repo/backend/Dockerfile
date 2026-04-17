# syntax=docker/dockerfile:1.6
#
# Multi-stage build:
#   * base    — module cache + source tree; used by both test and build
#   * test    — runs `go test -cover ./...` (targeted by run_tests.sh)
#   * build   — produces static server + seed binaries
#   * runtime — slim alpine image with the entrypoint that waits for
#               Postgres, applies migrations, seeds, then runs the server

FROM golang:1.22-alpine AS base
ENV GOFLAGS="-mod=mod" CGO_ENABLED=0
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# -- test stage --------------------------------------------------------
FROM base AS test
# Default command runs the full suite with coverage. run_tests.sh may
# override via `command:` in docker-compose.test.yml. We do not enable
# `-race` by default because CGO is off in this stage to keep the
# layer small; local development can run `go test -race ./...`
# directly with CGO enabled.
CMD ["go", "test", "-cover", "./..."]

# -- build stage -------------------------------------------------------
FROM base AS build
RUN go build -ldflags="-s -w" -o /out/server ./cmd/server \
 && go build -ldflags="-s -w" -o /out/seed   ./cmd/seed \
 && go build -ldflags="-s -w" -o /out/keygen ./cmd/keygen

# -- runtime stage -----------------------------------------------------
FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates postgresql-client tini
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/seed   /app/seed
COPY --from=build /out/keygen /app/keygen
COPY migrations               /app/migrations
COPY docker-entrypoint.sh     /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
EXPOSE 8080
# tini reaps zombies and forwards signals so graceful shutdown works.
# Operators can override the entrypoint to run ad-hoc utilities, e.g.:
#   docker compose run --rm --entrypoint /app/keygen backend
ENTRYPOINT ["/sbin/tini", "--", "/app/docker-entrypoint.sh"]
