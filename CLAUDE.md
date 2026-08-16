# ai-ssh-tools

Go MCP server giving AI agents an SSH client: connection pooling, command sanitisation, SFTP, local port forwarding, system vitals, and supervised background processes with a Git safety net.

Module: `github.com/khalidelmerrah/ai-ssh-tools`. Go 1.25.

## Commands

```bash
go build ./cmd/...
go test ./...
go test ./cmd/... -run TestProfileWeakeningRejected -v
./build.sh                 # build for the host platform
BUILD_ALL=1 ./build.sh     # cross-compile all targets into dist/ + SHA256SUMS
./build.ps1 -All           # same, on Windows
```

## Layout

One flat `package main` in `cmd/ai-ssh-tools/` - there are no internal packages, so grep is the index. What lives where:

| File | Contents |
|---|---|
| `main.go` | MCP tool + prompt registration, server instructions, version string |
| `handlers.go` | Every MCP tool handler and its typed args struct (~2k lines) |
| `cli.go` | Standalone CLI subcommands (`exec`, `vitals`, `transfer`, ...) |
| `config.go` | `HostProfile`, profile registry, `loadProfiles`/`saveProfile` |
| `security.go` | `sanitiseCommand`, `shellQuote`, profile-write policy |
| `pool.go` | Connection pool, TOFU host keys, circuit breaker |
| `audit.go` `ratelimit.go` `tunnels.go` `process.go` | Small single-purpose helpers |
| `sshconfig.go` `vitals.go` `prompts.go` | `~/.ssh/config` parsing, vitals parsing, MCP prompts |
| `pipe_windows.go` / `pipe_other.go` | Build-tagged SSH agent transport |

`dist/` holds release artifacts, gitignored, produced only by `BUILD_ALL=1` / `-All`.

## State and environment

- `ssh_hosts.json` is read from **the binary's own directory**, not the working directory. Override with `SSH_HOSTS_PATH` - that is how tests point at a temp file.
- `AI_SSH_ALLOW_PROFILE_WRITES=1` enables `save_ssh_profile` over MCP. Unset by default.
- `SSH_AUTH_SOCK` selects the agent on Unix; Windows uses the named pipe in `pipe_windows.go`.
- Runtime state lives in `~/.ai-ssh-tools/`: `known_hosts.json` (TOFU fingerprints) and `audit.log`.

## Testing

Handlers are tested directly, without a server. Isolate state with the existing helpers rather than inventing new ones:

- `setupTempHome(t)` redirects `HOME`/`USERPROFILE` so TOFU and audit writes land in a temp dir.
- `t.Setenv("SSH_HOSTS_PATH", …)` plus `loadProfiles()` gives a clean profile registry.
- `RegisterProfileForTest` / `DeleteProfileForTest` inject a profile without touching disk.
- `startMockSSHServer(t)` provides a real SSH listener; its exec handler hangs deliberately, for timeout tests.

A handler returning a user-facing failure sets `result.IsError` and returns a `nil` error - a non-nil third return means an internal server fault. Assert on `IsError`, not `err`.

## Releasing

Binaries are **never committed**. Tagging `v*` runs `.github/workflows/release.yml`, which tests, cross-compiles into `dist/`, generates `SHA256SUMS`, and uploads both to GitHub Releases. A release is a version bump in `main.go` + a `CHANGELOG.md` entry + a tag - nothing else.

## Gotchas

- `sanitiseCommand` is an anti-chaining filter, not a harm filter: pipes, redirections, and `rm -rf /` all pass. `allowed_commands` is the real boundary. Any change to what gets accepted needs a matching test.
- Anything interpolated into a remote shell command must go through `shellQuote()`.
- `save_ssh_profile` is denied over MCP unless `AI_SSH_ALLOW_PROFILE_WRITES=1`, and may never loosen an existing profile. The profile is where every guardrail lives - do not add a path that writes one without going through `checkProfileWeakening`.
- Connection pooling means a mutated session leaks into later calls. Reset state rather than assuming a fresh connection per tool call.
- A new MCP tool is three edits, all required: an args struct with `jsonschema` tags plus the handler in `handlers.go`, and an `mcp.AddTool` call in `main.go`. A handler with no registration is silently dead.
- `gofmt -l` is already dirty on `audit.go`, `vitals.go` and `main.go` (pre-existing struct-field alignment). Do not reformat them as a drive-by - it buries the real diff. Fix it in its own commit or leave it.
- Never commit `ssh_hosts.json` or `.server-creds`; both are gitignored. Profile passwords are stored in cleartext, so prefer `key_path` or `use_agent` in anything you write or document.
