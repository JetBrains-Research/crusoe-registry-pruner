#!/usr/bin/env bash
set -euo pipefail

if [ "${AIR_STARTUP_MODE:-}" = warmup ]; then WARMUP=1; else WARMUP=; fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_DIR"

BUILDX_VERSION=v0.30.1

log() { echo "[startup] $*"; }

# Asserts the Go toolchain, module cache and build all work as a real task needs.
healthcheck() {
  log "healthcheck: vet + build + test"
  until go vet ./... && go build -o /tmp/crusoe-registry-pruner . && go test ./...; do
    log "healthcheck: not ready yet, retrying in 5s"
    sleep 5
  done
  log "healthcheck: OK ($(go version))"
}

log "go toolchain: $(go version)"

log "downloading Go modules"
go mod download
log "modules downloaded"

if [ -n "$WARMUP" ]; then
  log "priming build cache"
  go build ./...
  if docker info >/dev/null 2>&1; then
    if ! docker buildx version >/dev/null 2>&1; then
      log "installing docker buildx ${BUILDX_VERSION}"
      mkdir -p "$HOME/.docker/cli-plugins"
      curl -fsSL -o "$HOME/.docker/cli-plugins/docker-buildx" \
        "https://github.com/docker/buildx/releases/download/${BUILDX_VERSION}/buildx-${BUILDX_VERSION}.linux-amd64"
      chmod +x "$HOME/.docker/cli-plugins/docker-buildx"
    fi
    log "building Docker image crusoe-registry-pruner:dev"
    # Build steps need the egress proxy (when set) to reach proxy.golang.org.
    DOCKER_BUILDKIT=1 docker build --network host \
      --build-arg HTTP_PROXY --build-arg HTTPS_PROXY --build-arg NO_PROXY \
      -t crusoe-registry-pruner:dev . || log "WARNING: docker build failed (non-fatal)"
  else
    log "docker daemon not reachable; skipping image build"
  fi
  healthcheck
fi

log "done"
