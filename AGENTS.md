# Agent Guidelines

## Build and verification

- Use Go 1.27.2 (see [go.mod](go.mod) and [mise.toml](mise.toml)).
- Run focused tests with `go test ./internal/<package>` during changes; run `make all` (vet, tests, build) before finishing. See [Makefile](Makefile) and [CI](.github/workflows/ci.yml) for the authoritative checks.
- Build with `make build` rather than a bare `go build`: the Makefile sets `CGO_ENABLED=0`, version/linker flags, and a purego compiler flag needed for FreeBSD/NetBSD. Use `make race` for race checks (it enables cgo), and `make cross` when changing platform-sensitive code.

## Ownership

- [cmd/mp3blasterr](cmd/mp3blasterr) wires startup, configuration, audio backend, and session persistence.
- [internal/app](internal/app) owns behavior and state independently of the terminal toolkit; [internal/ui](internal/ui) renders that state and routes terminal input. Keep behavior in app and presentation in ui.
- [internal/playback](internal/playback) chooses tracks and maintains play history; [internal/audio](internal/audio) handles decoding and device output. Use the playback `Backend`/`Track` interfaces across that boundary.
- [internal/config](internal/config) owns settings and persistence; [internal/playlist](internal/playlist) owns playlist structure; [internal/filemanager](internal/filemanager) owns directory browsing; [internal/keys](internal/keys) owns key commands and modes.

## Concurrency and tests

- Call `playback.Controller` methods only on the UI event-loop goroutine. Background work must report back through the provided post callback; never call `app.View.Post` from the UI goroutine because it blocks until the callback runs.
- Add focused package-local tests near the behavior changed. Existing tests use standard `testing`, fakes and `t.TempDir()`; UI drawing tests use tcell's `SimulationScreen` (see [screen_test.go](internal/ui/screen_test.go)).
- Consult [help.txt](internal/app/help.txt) for user-facing commands and [config.sample.yaml](config.sample.yaml) for configuration examples; update them when those interfaces change.