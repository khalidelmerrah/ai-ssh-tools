# ai-ssh-tools Open Source Readiness Plan

Date: 2026-05-24
Project: ai-ssh-tools
Status: Draft recommendations

## Purpose

`ai-ssh-tools` is a free and open-source local MCP server that lets AI assistants operate remote servers over SSH without exposing credentials to the model context.

The project idea is strong. SSH is inherently local, sensitive, and environment-specific, so a local Go binary is a good foundation. The current implementation has the right core ingredients: named profiles, credential masking, connection pooling, SFTP operations, command policy controls, audit logs, process management, and port forwarding.

The project should now move from "powerful prototype" to "trusted open-source operations tool." That means better packaging, safer tool design, clearer architecture, production-grade tests, security hardening, and a documented release process.

## Product Positioning

This should not be marketed as "give an AI a shell." That framing is too broad and risky.

Recommended positioning:

> A local MCP SSH operations bridge for AI workbenches. It lets assistants inspect, deploy, transfer files, read logs, manage processes, and open tunnels through named SSH profiles while keeping credentials local.

Target users:

- Developers managing personal VPS machines.
- Small teams that want AI-assisted server operations.
- DevOps users who want local control instead of a hosted SSH proxy.
- Open-source maintainers who want a transparent alternative to closed remote agents.

Non-goals:

- Do not become a hosted SSH service by default.
- Do not store user secrets in a cloud account.
- Do not replace real access control, deployment pipelines, or server monitoring.
- Do not encourage unconstrained command execution as the primary experience.

## Current Direction Review

The direction is mostly correct.

Good decisions:

- Go binary is a good fit for local distribution.
- Local stdio MCP is suitable for private SSH keys and local network access.
- Named profiles reduce credential leakage in model conversations.
- Connection pooling makes repeated operations fast.
- SFTP tools are better than forcing all file operations through shell commands.
- Audit logging is important for trust.
- Read-only profiles and command allowlists are the right idea.
- Git snapshot wrapping is useful for remote file changes and deploy experiments.

Areas that need improvement:

- The tool surface is still too shell-centric.
- Internal packages are not separated enough for long-term maintenance.
- Release binaries should be managed through release artifacts, not loose working-tree files.
- There is no changelog, version workflow, or open-source governance documentation.
- Production-grade testing needs a real SSH integration test environment.
- Several security boundaries need to be tightened before public recommendation.

## Critical MCP Startup Issue

### Error

After adding the tool as an MCP server, initialization failed with:

```text
2026/05/24 12:57:28 [ai-ssh-tools] [info] loaded 3 SSH profile(s)
panic: AddTool: tool "connect_and_execute": input schema: ForType(main.ConnectAndExecuteArgs): tag must not begin with 'WORD=': "description=Named profile alias from ssh_hosts.json (mutually exclusive with host/user)"
goroutine 1 [running]:
github.com/modelcontextprotocol/go-sdk/mcp.AddTool[...](...)
github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp/server.go:506 +0xd9
main.main()
github.com/khalidelmerrah/ai-ssh-tools/cmd/ai-ssh-tools/main.go:45 +0x21a
: calling "initialize": EOF.
```

### Meaning

This was not an SSH profile problem. The server loaded profiles successfully, then crashed while registering the MCP tool schemas.

Root cause:

- The Go MCP SDK generated JSON Schema from tool input structs.
- One or more struct tags used a format like `description=...`.
- The schema parser in the dependency rejected tags beginning with `WORD=`.
- Because tool registration happens during startup, the process panicked before MCP initialization completed.
- The MCP client then saw `initialize: EOF` because the server process exited.

### Required Fix

Use schema tags supported by the current Go MCP SDK and `jsonschema-go` dependency.

Recommended pattern:

```go
Profile string `json:"profile,omitempty" jsonschema:"Named profile alias from ssh_hosts.json"`
```

Avoid this pattern:

```go
Profile string `json:"profile,omitempty" jsonschema:"description=Named profile alias from ssh_hosts.json"`
```

After fixing tags:

1. Rebuild the binary.
2. Confirm the MCP config points at the rebuilt binary.
3. Run the server manually once to confirm it does not panic.
4. Test with MCP Inspector or an MCP client initialization.
5. Add a regression test or CI startup smoke test so schema tag regressions fail before release.

### Acceptance Criteria

- Starting the server logs profile loading and reaches stdio transport startup.
- MCP initialization succeeds in Claude, Gemini, Codex, or MCP Inspector.
- All tools are listed without schema generation errors.
- CI includes a startup/schema smoke test.

## Recommended Tool Design

The current one-tool-per-action approach is correct because the server exposes fewer than 15 core actions.

Keep these low-level tools:

- `connect_and_execute`
- `secure_file_delta`
- `secure_file_transfer`
- `ssh_port_forward`
- `manage_remote_process`
- `get_system_vitals`
- `git_rollback`
- `list_profiles`
- `save_ssh_profile`

Add safer high-level tools:

- `check_service_status`
- `restart_service`
- `tail_service_logs`
- `inspect_disk_usage`
- `inspect_memory`
- `inspect_ports`
- `deploy_git_repo`
- `run_health_check`
- `verify_web_endpoint`
- `list_remote_directory`
- `read_remote_file`
- `write_remote_file`

Why:

- AI assistants perform better with intent-based tools.
- Users understand the risk of `restart_service` more clearly than a raw shell command.
- Tool descriptions can include read/write/destructive metadata.
- Safer defaults reduce the need for broad command allowlists.

Recommended model:

- Keep raw command execution for advanced users.
- Prefer high-level tools in documentation and examples.
- Mark destructive tools clearly.
- Make read-only profiles compatible with all read-only tools.

## Architecture Recommendations

Current structure is simple but too concentrated in `cmd/ai-ssh-tools`.

Recommended package layout:

```text
cmd/ai-ssh-tools/
  main.go

internal/config/
  profiles.go
  known_hosts.go

internal/sshpool/
  pool.go
  auth.go
  hostkey.go

internal/security/
  command_policy.go
  path_policy.go
  shellquote.go

internal/tools/
  execute.go
  files.go
  transfer.go
  processes.go
  tunnels.go
  vitals.go
  profiles.go

internal/audit/
  audit.go

internal/mcpserver/
  server.go
  schemas.go

internal/testutil/
  ssh_server.go
```

Benefits:

- Easier tests without starting the whole MCP server.
- Clearer boundaries between MCP, SSH, SFTP, and security logic.
- Safer future refactors.
- Cleaner public contribution path.

## Security Requirements

These are required before recommending the tool publicly.

### Command Execution

- Block newline and carriage return command separators.
- Decide whether single pipe `|` should be allowed. If allowed, document the risk and require explicit profile policy.
- Quote all remote shell arguments using a single shared helper.
- Never interpolate `workdir`, file paths, process IDs, or service names into shell commands without validation and quoting.
- Validate `workdir` as an absolute path where appropriate.
- Reject control characters in commands and paths.
- Add explicit max command length.
- Add explicit timeout defaults and per-profile timeout limits.

### Workdir and Git Safety

- Quote `workdir` everywhere it is used in `cd`.
- Ensure `git_rollback` cannot operate outside the intended repository.
- Avoid `git clean -fd` unless the user or profile explicitly allows destructive cleanup.
- Add a dry-run or preview mode for rollback.
- Record rollback actions in the audit log.

### Profile Security

- Validate aliases, hostnames, usernames, and ports when saving profiles.
- Support SSH agent properly if `use_agent` is exposed.
- If `use_agent` is not implemented, remove it from config until it is.
- Do not return sensitive fields from `list_profiles`.
- Ensure private key paths and passwords never appear in errors returned to the MCP client.
- Add profile-level permissions for each tool category.

### Host Key Verification

- Keep strict host key verification.
- Document TOFU behavior clearly.
- Provide a command or tool to list saved known hosts.
- Provide a safe way to rotate a known host fingerprint.
- Warn clearly when first-use trust occurs.

### SFTP and File Safety

- Enforce allowed paths on every file operation.
- Apply allowed path checks to process log directories when relevant.
- Normalize remote paths consistently using POSIX path rules.
- Reject relative paths for tools that claim to require absolute paths.
- Add file size caps for reads and downloads.
- Add overwrite behavior controls for writes and uploads.

### Process Management

- Store process state under a predictable but profile-scoped directory.
- Prevent process ID collisions.
- Use random IDs instead of timestamp-only IDs.
- Validate and quote all paths in generated scripts.
- Capture stdout, stderr, pid, exit code, start time, and command hash.
- Add process cleanup tools.

### Port Forwarding

- Bind local forwards to `127.0.0.1` by default.
- Require explicit opt-in for non-localhost binds.
- Add profile restrictions for allowed remote hosts and ports.
- Audit start and stop events.
- Add tunnel idle timeout.

### Audit Logging

- Keep audit logs local.
- Include timestamp, profile, tool, action, status, duration, and error class.
- Do not log secrets.
- Avoid logging full command output.
- Consider hashing commands in addition to storing sanitized command text.
- Add log rotation.

### Open Source Security Process

- Add `SECURITY.md`.
- Add responsible disclosure contact.
- Add supported versions policy.
- Add dependency vulnerability scanning.
- Add secret scanning.
- Add signed release checksums.

## Production-Grade Testing Requirements

### Unit Tests

Required coverage:

- Command sanitizer.
- Shell quoting helper.
- Workdir validation.
- Path allowlist and traversal cases.
- Profile parsing and validation.
- Host key matching.
- Rate limiter behavior.
- Audit redaction.
- Process ID validation.
- Schema tag compatibility.

Important negative test cases:

```text
echo ok
echo ok && rm -rf /
echo ok; rm -rf /
echo ok
rm -rf /
echo $(id)
echo `id`
cd /tmp && whoami
/tmp/app; rm -rf /
/tmp/app
whoami
../../etc/passwd
/var/log/../../etc/shadow
```

### MCP Startup Tests

Add a test that builds or starts the server and verifies:

- It does not panic while registering tools.
- Tool schema generation succeeds.
- The server can answer MCP `initialize`.
- The tool list contains all expected tools.

### Integration Tests

Use a local containerized SSH server for integration tests.

Test scenarios:

- Password login.
- Key login.
- Passphrase-protected key login.
- SSH agent login, if supported.
- Host key pinning.
- TOFU first connection.
- TOFU mismatch rejection.
- Command allowlist and blocklist.
- Read-only profile behavior.
- SFTP read, write, list, upload, download.
- Allowed path enforcement.
- Background process start, status, logs, stop.
- Port forwarding to a local test service.
- Git wrapped execution and rollback.

### End-to-End MCP Tests

Use MCP Inspector and at least one real client.

Required clients:

- Claude Desktop or Claude Code.
- Gemini CLI or Gemini extension if supported.
- Codex local MCP config if supported.

Test cases:

- Add the MCP server to config.
- Initialize successfully.
- List tools.
- Call `list_profiles`.
- Run one read-only command.
- Read one allowed file.
- Confirm blocked operations fail safely.

### CI Requirements

GitHub Actions should run:

- `go test ./...`
- `go vet ./...`
- `go test -race ./...`
- static analysis with `staticcheck`
- dependency vulnerability scan with `govulncheck`
- secret scanning with `gitleaks` or equivalent
- cross-platform build for Linux, macOS, and Windows
- release artifact checksum generation

### Manual Release Testing

Before every release:

- Test Windows binary on Windows.
- Test Linux binary on Linux.
- Test macOS binaries if possible.
- Test with an actual MCP client.
- Test a fresh install with no `ssh_hosts.json`.
- Test upgrade from previous version.
- Test a broken config file.
- Test missing private key.
- Test wrong host key.

## Open Source Project Requirements

Add these files:

- `CHANGELOG.md`
- `SECURITY.md`
- `CONTRIBUTING.md`
- `CODE_OF_CONDUCT.md`
- `LICENSE`
- `docs/installation.md`
- `docs/configuration.md`
- `docs/security-model.md`
- `docs/testing.md`
- `docs/mcp-client-setup.md`
- `docs/troubleshooting.md`
- `examples/ssh_hosts.example.json`

Recommended license:

- MIT or Apache-2.0 for maximum adoption.
- Apache-2.0 if explicit patent protection matters.

Recommended repository policy:

- Free forever for local use.
- No telemetry by default.
- No cloud dependency.
- No secret collection.
- Transparent security model.
- Public roadmap.
- Clear contribution guidelines.

## Packaging and Distribution

Recommended distribution channels:

- GitHub Releases with signed checksums.
- Homebrew tap for macOS and Linux.
- Scoop or Winget package for Windows.
- Docker image only for testing, not primary SSH key usage.
- Optional install script that downloads the right binary.

Release artifacts:

```text
ai-ssh-tools-linux-amd64
ai-ssh-tools-linux-arm64
ai-ssh-tools-darwin-amd64
ai-ssh-tools-darwin-arm64
ai-ssh-tools-windows-amd64.exe
checksums.txt
checksums.txt.sig
```

Do not commit generated binaries to the normal source tree.

## Versioning and Changelog

Adopt semantic versioning.

Recommended current baseline:

- If this is already publicly used, start at `0.2.0` or `0.3.0`.
- If it is not released yet, use `0.1.0`.
- Reserve `1.0.0` for stable config format, documented security model, full release process, and production-grade tests.

Every release should include:

- Version number.
- Date.
- Commit hash.
- Added, Changed, Fixed, Security, Removed sections.
- Migration notes if config format changes.

## Documentation Requirements

README should be reorganized around user workflow:

1. What this tool does.
2. Who it is for.
3. Security model.
4. Install.
5. Configure a profile.
6. Add to MCP client.
7. Test with `list_profiles`.
8. Common workflows.
9. Troubleshooting.
10. Development and contributing.

Important docs to add:

- MCP setup for Claude.
- MCP setup for Gemini.
- MCP setup for Codex.
- Windows-specific notes.
- Host key setup guide.
- Read-only profile examples.
- Safe command allowlist examples.
- How audit logs work.
- How to rotate known hosts.
- How to recover from a bad profile.

## Configuration Recommendations

Current JSON profile config is fine for a first version.

Recommended improvements:

- Provide `ssh_hosts.example.json`.
- Validate config at startup and report all errors together.
- Add a `validate_config` tool or CLI command.
- Support comments through a separate `.jsonc` format only if parser support is deliberate.
- Add profile permissions:

```json
{
  "alias": "prod-web",
  "host": "203.0.113.10",
  "port": 22,
  "user": "deploy",
  "key_path": "~/.ssh/id_ed25519",
  "host_key": "SHA256:...",
  "readonly": false,
  "permissions": {
    "execute": true,
    "sftp_read": true,
    "sftp_write": false,
    "port_forward": false,
    "process_start": false,
    "git_rollback": false
  },
  "allowed_commands": [
    "^systemctl status [a-zA-Z0-9_.@-]+$",
    "^journalctl -u [a-zA-Z0-9_.@-]+ -n [0-9]+ --no-pager$"
  ],
  "allowed_paths": [
    "/var/log",
    "/home/deploy/app"
  ]
}
```

## UX Improvements

Add a first-run experience:

- Detect missing config.
- Print where config should live.
- Provide example profile.
- Provide exact MCP client config snippet.
- Provide a `--validate-config` command.
- Provide a `--list-tools` or `--doctor` command.

Add a doctor command:

```text
ai-ssh-tools doctor
```

It should check:

- Config exists.
- Config parses.
- Profiles validate.
- Key files exist.
- Known hosts file is readable.
- Audit log directory is writable.
- MCP schema generation works.

## Priority Roadmap

### Phase 1: Stability and Startup

- Fix MCP schema tag panic.
- Add startup/schema regression test.
- Add `CHANGELOG.md`.
- Add version constant shared by binary and MCP server metadata.
- Add `--version`.
- Add `--validate-config`.

### Phase 2: Security Hardening

- Add shell quoting helper.
- Quote and validate all workdir usage.
- Block newline and carriage return in commands.
- Add path validation tests.
- Implement or remove SSH agent support.
- Improve audit redaction.
- Add `SECURITY.md`.

### Phase 3: Test Infrastructure

- Add containerized SSH integration tests.
- Add MCP Inspector smoke test documentation.
- Add GitHub Actions CI.
- Add race tests.
- Add vulnerability scanning.

### Phase 4: Productization

- Split internal packages.
- Add docs directory.
- Add example config.
- Add install scripts.
- Add release workflow.
- Generate checksums.
- Publish first pre-1.0 release.

### Phase 5: Safer High-Level Tools

- Add service status and logs tools.
- Add deploy helper tool.
- Add endpoint verification tool.
- Add disk and memory inspection tools.
- Update README examples to prefer high-level tools.

## Definition of Done for Public Release

The project is ready for serious public use when:

- MCP initialization works reliably across supported clients.
- The startup panic class is covered by tests.
- Raw shell execution has strong validation and documented limits.
- Workdir and path handling are safe and tested.
- SSH auth modes are either implemented or not advertised.
- Read-only mode is enforced consistently.
- CI runs tests, race checks, vet, static analysis, and vulnerability scans.
- Release artifacts are produced by CI.
- Checksums are published.
- README explains the security model honestly.
- `SECURITY.md`, `CONTRIBUTING.md`, `CHANGELOG.md`, and license files exist.
- At least one full integration test uses a real SSH server.
- At least one real MCP client has been manually verified.

## Final Opinion

The project is worth continuing.

The core idea is useful and well matched to a local Go MCP server. The strongest version of this project is a free, open-source, local-first SSH operations bridge for AI assistants. Keep the local binary, named profiles, audit logs, SFTP support, process tools, tunnel support, and host key verification. Improve the tool design, architecture, release process, security boundaries, and test depth before calling it production-ready.

