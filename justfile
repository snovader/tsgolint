#!/usr/bin/env -S just --justfile

set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]
set shell := ["bash", "-cu"]

alias r := ready

ready:
  just fmt
  just lint
  just test

[unix]
init:
  node tools/prepare-compiler.mjs
  pnpm install
  cd e2e && pnpm --ignore-workspace install && cd ..

[windows]
init:
  node tools/prepare-compiler.mjs
  pnpm install
  Set-Location e2e; pnpm --ignore-workspace install; Set-Location ..

[unix]
build:
  go build -ldflags="-s -w" -trimpath -o tsgolint ./cmd/tsgolint

[windows]
build:
  $env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -trimpath -o tsgolint.exe ./cmd/tsgolint

test: build
  cd e2e && pnpm --ignore-workspace run test --run && cd ..
  go test ./internal/...

update-snaps:
  UPDATE_SNAPS=true go test ./internal/...

lint:
  golangci-lint run

fmt:
  gofmt -w internal cmd tools
  pnpm run fmt

shim:
  go run tools/gen_shims/main.go

pull:
  node tools/prepare-compiler.mjs
