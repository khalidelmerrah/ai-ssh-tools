# Changelog

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

