# ai-ssh-tools

Go MCP server giving AI agents an SSH client: connection pooling, command sanitisation, SFTP, local port forwarding, system vitals, and supervised background processes with a Git safety net.

Module: `github.com/khalidelmerrah/ai-ssh-tools`. Go 1.25.

## Commands

```bash
go build ./cmd/...
go test ./...
./build.sh          # cross-compile all targets (Linux/macOS/Windows, amd64+arm64)
./build.ps1         # same, on Windows
```

## Layout

- `cmd/` - entry points
- Prebuilt binaries (`ai-ssh-tools-*`) are committed at the repo root. Rebuild them with the build script when shipping a release; do not hand-edit.

## Gotchas

- Command sanitisation is a security boundary, not a convenience filter. Any change to what gets accepted needs a matching test.
- Connection pooling means a mutated session leaks into later calls. Reset state rather than assuming a fresh connection per tool call.
- Cross-compiled binaries and `CHANGELOG.md` are part of a release commit - a version bump without rebuilt binaries ships stale code.
