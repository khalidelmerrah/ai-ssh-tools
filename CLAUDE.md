# ai-ssh-tools

Go MCP server giving AI agents an SSH client: connection pooling, command sanitisation, SFTP, local port forwarding, system vitals, and supervised background processes with a Git safety net.

Module: `github.com/khalidelmerrah/ai-ssh-tools`. Go 1.25.

## Commands

```bash
go build ./cmd/...
go test ./...
./build.sh                 # build for the host platform
BUILD_ALL=1 ./build.sh     # cross-compile all targets into dist/ + SHA256SUMS
./build.ps1 -All           # same, on Windows
```

## Layout

- `cmd/` - entry points
- `dist/` - release artifacts, gitignored. Produced only by `BUILD_ALL=1` / `-All`.

## Releasing

Binaries are **never committed**. Tagging `v*` runs `.github/workflows/release.yml`, which tests, cross-compiles into `dist/`, generates `SHA256SUMS`, and uploads both to GitHub Releases. A release is a version bump in `main.go` + a `CHANGELOG.md` entry + a tag - nothing else.

## Gotchas

- `sanitiseCommand` is an anti-chaining filter, not a harm filter: pipes, redirections, and `rm -rf /` all pass. `allowed_commands` is the real boundary. Any change to what gets accepted needs a matching test.
- Anything interpolated into a remote shell command must go through `shellQuote()`.
- `save_ssh_profile` is denied over MCP unless `AI_SSH_ALLOW_PROFILE_WRITES=1`, and may never loosen an existing profile. The profile is where every guardrail lives - do not add a path that writes one without going through `checkProfileWeakening`.
- Connection pooling means a mutated session leaks into later calls. Reset state rather than assuming a fresh connection per tool call.
