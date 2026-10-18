ARG VERSION=dev

FROM golang:1.27 AS build

ARG VERSION

ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /go/src/app

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    go vet -v ./...; \
    go build -ldflags="-w -s -X crusoe-registry-pruner/internal/crusoe/utils.Version=${VERSION}" \
        -trimpath \
        -o /go/bin/crusoe-registry-pruner;

FROM gcr.io/distroless/static-debian13@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

ARG VERSION

LABEL \
    org.opencontainers.image.title="crusoe-registry-pruner" \
    org.opencontainers.image.description="Single-shot Go job that deletes stale manifests from Crusoe Container Registry (CCR)" \
    org.opencontainers.image.source="https://github.com/JetBrains-Research/crusoe-registry-pruner" \
    org.opencontainers.image.authors="ozren.dabic@jetbrains.com" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.version=$VERSION

USER 65532:65532

COPY --from=build --chmod=755 /go/bin/crusoe-registry-pruner /usr/local/bin/

CMD ["crusoe-registry-pruner"]
