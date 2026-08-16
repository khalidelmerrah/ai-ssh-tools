```
    _    ___        ____ ____  _   _       _____ ___   ___  _     ____  
   / \  |_ _|      / ___/ ___|| | | |     |_   _/ _ \ / _ \| |   / ___| 
  / _ \  | | _____ \___ \___ \| |_| | _____ | || | | | | | | |   \___ \ 
 / ___ \ | ||_____| ___) |__) |  _  ||_____|| || |_| | |_| | |___ ___) |
/_/   \_\___|      |____/____/|_| |_|       |_| \___/ \___/|_____|____/ 
```

# ai-ssh-tools

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![MCP Protocol](https://img.shields.io/badge/MCP-1.6.1-22c55e?style=flat)](https://modelcontextprotocol.io)
[![Release](https://img.shields.io/github/v/release/khalidelmerrah/ai-ssh-tools?color=orange&style=flat)](https://github.com/khalidelmerrah/ai-ssh-tools/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)

A lightweight, production-grade **CLI tool and Model Context Protocol (MCP) server** written in Go. Gives AI agents (Claude, Cursor, Antigravity) and developers safe, zero-latency SSH operations with connection pooling, credential isolation, SFTP streaming, system diagnostics, Docker/service management, and automated Git safety-net rollbacks.

---

### ⚡ Quick Navigation
- [🚀 Quick Start (CLI & MCP)](#-quick-start)
- [📦 Installation (All OS)](#-installation)
- [🤖 AI Agent Configuration](#-ai-agent-configuration)
- [🛠️ MCP Tools (12 Tools)](#️-mcp-tools-reference)
- [🔒 Security & Context Protection](#-security--guardrails)
- [⚙️ Profile Config & Rules](#-configuration--agent-rules)

---

## 🚀 Quick Start

No pre-configuration required. Connect dynamically on the fly:

```bash
# Execute remote command
ai-ssh-tools exec --host 192.168.1.50 --user deploy "uptime"

# Run with sudo and PTY
ai-ssh-tools exec --host 192.168.1.50 --user deploy --sudo "systemctl restart nginx"

# Instant system vitals (or --json for automation)
ai-ssh-tools vitals --host 192.168.1.50 --user deploy

# Inspect Docker containers
ai-ssh-tools docker --host 192.168.1.50 --user deploy

# Manage systemd service
ai-ssh-tools service --host 192.168.1.50 --user deploy --name nginx --action status

# High-throughput SFTP streaming
ai-ssh-tools transfer --host 192.168.1.50 --user deploy --src ./app.tar.gz --dst /opt/app/app.tar.gz --op upload

# Start as MCP Server for AI assistants
ai-ssh-tools serve
```

---

## 📦 Installation

<details>
<summary><b>Option 1: <code>go install</code> (Recommended)</b></summary>

```bash
go install github.com/khalidelmerrah/ai-ssh-tools/cmd/ai-ssh-tools@latest
```
</details>

<details>
<summary><b>Option 2: Pre-compiled Binaries (macOS / Linux / Windows)</b></summary>

#### 🍎 macOS
```bash
# Apple Silicon (arm64):
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-darwin-arm64 && chmod +x ai-ssh-tools && sudo mv ai-ssh-tools /usr/local/bin/

# Intel (amd64):
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-darwin-amd64 && chmod +x ai-ssh-tools && sudo mv ai-ssh-tools /usr/local/bin/
```

#### 🐧 Linux
```bash
# x86_64 / amd64:
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-linux-amd64 && chmod +x ai-ssh-tools && sudo mv ai-ssh-tools /usr/local/bin/

# ARM64:
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-linux-arm64 && chmod +x ai-ssh-tools && sudo mv ai-ssh-tools /usr/local/bin/
```

#### 🪟 Windows (PowerShell)
```powershell
$dir = "$env:LOCALAPPDATA\ai-ssh-tools"; New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest -Uri "https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-windows-amd64.exe" -OutFile "$dir\ai-ssh-tools.exe"
if ([Environment]::GetEnvironmentVariable("Path", "User") -notlike "*$dir*") {
    [Environment]::SetEnvironmentVariable("Path", "$([Environment]::GetEnvironmentVariable('Path', 'User'));$dir", "User")
}
```
</details>

---

## 🤖 AI Agent Configuration

Add `ai-ssh-tools` to your AI assistant with the stdio command: `ai-ssh-tools serve`.

<details>
<summary><b>Claude Desktop</b></summary>

Add to `claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "ai-ssh-tools": {
      "command": "ai-ssh-tools",
      "args": ["serve"]
    }
  }
}
```
*Config Path: `%APPDATA%\Claude\claude_desktop_config.json` (Windows) | `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)*
</details>

<details>
<summary><b>Cursor IDE</b></summary>

1. Go to **Cursor Settings** (`Ctrl+,`) > **Features** > **MCP Servers**.
2. Click **+ Add New MCP Server**.
3. Set **Type**: `command` | **Command**: `ai-ssh-tools serve`
</details>

<details>
<summary><b>Antigravity / Gemini CLI</b></summary>

Add to `antigravity.json` or `~/.gemini/antigravity/mcp_servers.json`:
```json
{
  "mcpServers": {
    "ai-ssh-tools": {
      "command": "ai-ssh-tools",
      "args": ["serve"]
    }
  }
}
```
</details>

<details>
<summary><b>Windsurf, Cline, Roo Code, Continue.dev & Zed</b></summary>

* **Windsurf** (`~/.codeium/windsurf/mcp_config.json`):
  ```json
  {"mcpServers": {"ai-ssh-tools": {"command": "ai-ssh-tools", "args": ["serve"]}}}
  ```
* **Cline / Roo Code** (`mcp_settings.json`):
  ```json
  {"mcpServers": {"ai-ssh-tools": {"command": "ai-ssh-tools", "args": ["serve"]}}}
  ```
* **Continue.dev** (`~/.continue/config.json`):
  ```json
  {"experimental": {"modelContextProtocolServers": [{"transport": {"type": "stdio", "command": "ai-ssh-tools", "args": ["serve"]}}]}}
  ```
* **Zed Editor** (`~/.config/zed/settings.json`):
  ```json
  {"context_servers": {"ai-ssh-tools": {"command": "ai-ssh-tools", "args": ["serve"]}}}
  ```
</details>

---

## 🛠️ MCP Tools Reference

`ai-ssh-tools` exposes **12 structured MCP tools**:

| Tool | Description | Parameters |
|---|---|---|
| `connect_and_execute` | Execute single atomic command with optional Git snapshot wrap | `host`, `user`, `command`, `workdir`, `git_wrapped`, `sudo`, `pty` |
| `get_system_vitals` | Return structured OS name, uptime, load avg, memory & disk in JSON | `host`, `user`, `port`, `profile` |
| `docker_containers` | Inspect running & stopped Docker containers (JSON output) | `host`, `user`, `all`, `profile` |
| `manage_service` | Manage systemd units (`status`, `start`, `stop`, `restart`, `logs`) | `host`, `user`, `name`, `action`, `lines`, `sudo` |
| `tail_remote_file` | Read trailing lines from remote log files | `host`, `user`, `path`, `lines` |
| `secure_file_delta` | Read (capped at 128KB), write, or list files via SFTP | `host`, `user`, `operation` (`read`/`write`/`list`), `path`, `content` |
| `secure_file_transfer` | Stream large files and binaries via SFTP buffer | `host`, `user`, `local_path`, `remote_path`, `direction` (`upload`/`download`) |
| `git_rollback` | Undo changes wrapped by automatic agent Git snapshots | `host`, `user`, `workdir`, `commits_back` |
| `manage_remote_process`| Supervise background tasks (`start`, `status`, `logs`, `stop`) | `host`, `user`, `action`, `command`, `process_id` |
| `ssh_port_forward` | Open/close local SSH port forwarding tunnels | `host`, `user`, `action`, `local_port`, `remote_port` |
| `list_profiles` | List loaded SSH connection profiles and metadata | *(none)* |
| `save_ssh_profile` | Dynamically save or update a profile in `ssh_hosts.json` | `alias`, `host`, `user`, `key_path`, `readonly`, etc. |

---

## 🔒 Security & Guardrails

<details>
<summary><b>🛡️ Security & Context Protection Details</b></summary>

1. **Local Credential Isolation**: Private keys and credentials stay local. Never logged or exposed in chat context.
2. **Context Window Protection**: Output exceeding **40 KB** or **400 lines** is automatically truncated (keeping head + tail preview) to prevent token exhaustion.
3. **Command Firewall**: Blocks shell chaining operators (`;`, `&&`, `||`, backticks, `$()`) to prevent injected compound payloads.
4. **TOFU Host Fingerprints**: SHA256 fingerprints recorded in `~/.ai-ssh-tools/known_hosts.json` prevent MITM attacks.
5. **Git Safety-Net**: `git_wrapped: true` creates pre/post execution commit checkpoints for 1-click rollback via `git_rollback`.
6. **Read-Only Profiles & Path Constraints**: `"readonly": true` and `"allowed_paths"` enforce strict access sandboxing.
7. **JSON Audit Log**: All events logged to `~/.ai-ssh-tools/audit.log` with zero secret exposure.
</details>

---

## ⚙️ Configuration & Agent Rules

<details>
<summary><b>Named Profiles Configuration (<code>ssh_hosts.json</code>)</b></summary>

Optional configuration for named aliases, command whitelists, and rate limits:

```json
[
  {
    "alias":            "prod-web",
    "host":             "203.0.113.10",
    "port":             22,
    "user":             "deploy",
    "key_path":         "~/.ssh/id_ed25519",
    "allowed_commands": ["^git.*$", "^systemctl status.*$"],
    "blocked_commands": [".*rm -rf.*"],
    "git_enabled":      true,
    "readonly":         false,
    "rate_limit_rpm":   60,
    "allowed_paths":    ["/var/log", "/home/deploy/app"]
  }
]
```
</details>

<details>
<summary><b>🧠 Recommended AI System Prompt / Rules</b></summary>

Add to `.cursorrules`, `CLAUDE.md`, or custom instructions:

```markdown
### Remote Server Operations (ai-ssh-tools):
- Connect dynamically using `host` and `user`. Authentication is automatic via ssh-agent / local keys.
- Submit single atomic commands per turn (no `;`, `&&`, `||` chaining).
- When modifying remote files in git repos, set `git_wrapped: true` with `workdir` for safety rollback.
- Use `get_system_vitals` or `docker_containers` for structured JSON metrics.
- For privileged commands, use `sudo: true`. For long tasks, use `manage_remote_process`.
```
</details>

<details>
<summary><b>🔨 Building from Source</b></summary>

```bash
go build -ldflags="-s -w" -o ai-ssh-tools ./cmd/ai-ssh-tools
go test -v ./...
./build.sh          # Linux/macOS cross-compile
pwsh .\build.ps1    # Windows cross-compile
```
</details>

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
