ARG VERSION=dev

FROM golang:1.26 AS build

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

FROM gcr.io/distroless/static-debian13:nonroot-165b5d63ce3528e18115acf8191122537154f238

USER 65532:65532

COPY --from=build --chmod=755 /go/bin/crusoe-registry-pruner /usr/local/bin/

CMD ["crusoe-registry-pruner"]
