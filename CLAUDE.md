# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

t1ms-go is a Starsiege: Tribes (Tribes 1) master server written in Go. It discovers and validates game servers by querying other master servers, responds to Tribes client server-list requests over UDP, and exposes a web API with server/master/stats data in XML and JSON formats.

## Build & Development Commands

- **Build:** `go build ./cmd/t1ms/`
- **Build all:** `go build ./...`
- **Test:** `go test -v ./...`
- **Lint:** `golangci-lint run --verbose` (CI uses v1.56.2)
- **Run:** `go run ./cmd/t1ms/ -c config.xml` (`-k` flag for service/daemon mode)

## Architecture

The project uses a standard Go project layout with `cmd/` and `internal/` packages:

- **cmd/t1ms/main.go** — Entry point, CLI flag parsing (`-c` config path, `-k` service mode), signal handling (SIGHUP reloads config), interactive console commands (`a`, `c`, `l`, `r`, `s`, `x`), and graceful shutdown via `context.Context`.
- **internal/config/** — XML config parsing/writing. `Config` struct uses `sync.RWMutex` for concurrent access. Provides `Load`, `Reload`, and `WriteDefault` functions.
- **internal/master/** — `Service` struct encapsulating UDP master server logic. Listens on configured address (default `:28000`), handles Tribes protocol packets (0x03 = server list query, 0x05 = heartbeat). Uses `sync.Map` for game servers, master servers, and clients. Includes rate limiting, server validation lifecycle (NotValidated -> Validated -> Expired -> deleted), and periodic packet building. Types are in `types.go`.
- **internal/web/** — `Service` struct encapsulating the HTTP server with embedded static files (`public/` via `//go:embed`). API endpoints under `/api/v1/` serve pre-built XML/JSON pages. The HTTP server uses proper timeouts and a local `ServeMux`. Includes `/api/v1/addserver` POST endpoint with input validation. Types are in `types.go`.

## Key Dependencies

- `github.com/TheKigen/t1net-go` — Tribes 1 network protocol library (packet reading/writing, server querying)

## Configuration

XML-based config file with `master-config` (listen address, MOTD, peer masters, rate limiting, query intervals) and `web-config` (HTTP listen address, page rebuild interval) sections. Config reloads on SIGHUP or console command `r`.

## CI

GitHub Actions workflow runs golangci-lint then tests on Go 1.21/1.22 across macOS, Ubuntu, and Windows.
