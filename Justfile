#!/usr/bin/env -S just --justfile

set default-list
set minimum-version := '1.55.0'
set shell := ['/bin/bash', '-c']

chart := 'chart/crusoe-registry-pruner'
image := 'ghcr.io/jetbrains-research/crusoe-registry-pruner'
version := `git describe --tags --always --dirty`
ldflags := '-X crusoe-registry-pruner/internal.Version=' + version

alias binary := build-binary
alias build := build-image
alias format := run-formatter
alias hook := run-hooks
alias lock := check-lockfile
alias run := run-pruner
alias setup := set-up-module
alias sync := sync-dependencies
alias template := render-chart
alias test := run-tests
alias tidy := tidy-lockfile
alias upgrade := upgrade-lockfile
alias vet := run-vet

build-binary:
    go build -ldflags='{{ ldflags }}' \
             -trimpath \
             -o bin/crusoe-registry-pruner \
             .

build-image *args:
    docker build --build-arg VERSION={{ version }} \
                 --tag {{ image }}:{{ version }} \
                 {{ args }} \
                 .

check-lockfile:
    go mod verify
    go mod tidy -diff

install-hooks:
    pre-commit install

lint: run-linter run-chart-linter

render-chart *args:
    helm template {{ chart }} {{ args }}

run-chart-linter:
    helm lint {{ chart }}

run-formatter: run-linter
    golangci-lint fmt

run-hooks:
    pre-commit run --all-files

run-linter:
    golangci-lint run --fix

run-pruner dry_run='true':
    set -a && source .env && set +a && \
    CRUSOE_PRUNER_DRY_RUN={{ dry_run }} go run -ldflags='{{ ldflags }}' .

run-tests target='./...' *args:
    go test -race {{ args }} {{ target }}

run-vet:
    go vet ./...

set-up-module: sync-dependencies install-hooks

sync-dependencies:
    go mod download

tidy-lockfile:
    go mod tidy

upgrade-lockfile:
    go get -u -t ./...
    go mod tidy
