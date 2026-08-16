# Changelog

## [2.0.0] - 2026-08-16 (commit: 5903895)
### Security
- **Shell injection fixed**: `workdir`, systemd service names, and file paths are now single-quote escaped via `shellQuote()` before interpolation into remote commands. Previously a value such as `/tmp; rm -rf /` was executed verbatim.
- **`save_ssh_profile` denied by default** (**BREAKING**): profile writes over MCP now require `AI_SSH_ALLOW_PROFILE_WRITES=1` in the server environment. Every other guardrail (readonly, allowed/blocked commands, allowed paths, rate limits, pinned host keys) lives in the profile, so an agent able to rewrite profiles could disarm all of them in a single call. The CLI path is unchanged.
- **Profile anti-weakening guard**: even with writes enabled, an existing profile may only be made stricter. Clearing `readonly`, emptying or widening `allowed_commands`/`allowed_paths`, dropping `blocked_commands` entries, raising or removing `rate_limit_rpm`, and changing a pinned `host_key` are all rejected and audit-logged.
- Audit log rotation at 10 MB, preventing unbounded growth of `~/.ai-ssh-tools/audit.log`.

### Fixed
- Read-only profiles no longer block read commands in `connect_and_execute`; the restriction now applies to git-wrapped writes only, making read-only profiles usable for their intended purpose.
- TOCTOU race in `ssh_port_forward`: the port conflict check and tunnel registration now happen under a single lock hold.
- Connection pool cleanup: pool is capped at 50 entries with a 10-minute idle reaper, replacing unbounded growth of persistent connections and keepalive goroutines.

### Changed
- **Documentation accuracy**: the command filter is documented as an *anti-chaining* filter, not a harm filter. Pipes, redirections, background operators, variable expansion, globs, and destructive single commands such as `rm -rf /` are not blocked — `allowed_commands` is the real restriction boundary. README, MCP server instructions, and tool descriptions updated to say so.
- Documented that `password` values in `ssh_hosts.json` are stored in cleartext.
- **Verifiable release artifacts**: `BUILD_ALL=1 ./build.sh` (and `./build.ps1 -All`) now collect cross-compiled binaries into `dist/` and emit a `SHA256SUMS` manifest, which the release workflow verifies and publishes alongside the binaries. README documents how to check a download. Binaries are never committed to the repository.
- Release workflow now runs `go test ./...` before building and fails on unmatched upload files. It previously uploaded `ai-ssh-tools-linux-amd64`, a filename `build.sh` never produced on the CI host — that asset was silently missing from releases.

### Removed
- **BREAKING**: `pkg/security`, `pkg/sshconfig`, and `pkg/vitals` deleted. They were verbatim copies of code in `cmd/ai-ssh-tools/`, never imported by the binary, and forced every fix to be applied twice.

## [1.1.0] - 2026-08-16
### Added
- **Dual-Mode CLI Interface**: Standalone subcommands (`exec`, `vitals`, `transfer`, `docker`, `service`, `tail`, `profiles`) with direct `--host`, `--user`, `--key`, `--sudo`, `--pty` flags without requiring profiles.
- **`~/.ssh/config` Support**: Automatically resolve hostnames, users, ports, and identity files from standard OpenSSH config.
- **SSH-Agent Support**: Automatic integration with running SSH agents via `SSH_AUTH_SOCK` (Linux/macOS) and Windows OpenSSH named pipes (`\\.\pipe\openssh-ssh-agent`).
- **Context Window Protection**: Smart head/tail output truncation preserving first 30 and last 100 lines for large outputs with informational summary banners.
- **Non-Interactive Sudo & PTY Support**: Added `--sudo` / `--pty` flags and parameters to safely inject passwords via stdin without hanging.
- **New MCP Tools**: Added `docker_containers`, `manage_service` (systemd), and `tail_remote_file`.

## [1.0.1] - 2026-05-24
### Added
- Initial open-source release preparation, security hardening, and test suites.

### Changed
- Bumped MCP server metadata version for release readiness.

