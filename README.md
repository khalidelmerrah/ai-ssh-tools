# ai-ssh-tools — SSH Operations Bridge for AI Workbenches & CLI

[![Go Version](https://img.shields.io/badge/go-1.25-blue.svg)](https://golang.org)
[![MCP Protocol](https://img.shields.io/badge/MCP-1.6.1-green.svg)](https://modelcontextprotocol.io)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A production-grade, modular, standalone **CLI tool and Model Context Protocol (MCP) server** written in Go. It empowers AI agents (Claude Desktop, Cursor, Antigravity, VS Code Copilot) and developers with safe, zero-latency SSH operations: connection pooling, credential isolation, SFTP file streaming, system vitals reporting, Docker container inspection, systemd service management, and an automated Git rollback safety-net.

---

## 🌟 Why `ai-ssh-tools`?

Giving AI assistants access to remote servers (VPS, cloud instances, staging servers) usually creates major friction:
* **The "PuTTY Copy-Paste" Pain**: Developers constantly copy terminal outputs, paste them into AI chats, wait for answers, and paste back.
* **Credential Leakage**: Pasting SSH keys, IPs, or passwords into chat prompts risks exposing secrets in model context logs.
* **Disasters Without Undo**: AI can execute unexpected commands or break configurations with no easy way to roll back.
* **Token Floods**: Commands like `cat /var/log/syslog` can dump megabytes of text and crash the LLM context window.

`ai-ssh-tools` eliminates these problems by acting as a **local operations bridge**:
1. **Local & Zero-Cloud**: Runs 100% on your local machine. Private keys never leave your device.
2. **Dual-Mode**: Works both as an instant terminal CLI tool and as a native MCP server for AI clients.
3. **Zero-Config Discovery**: Automatically connects using your active `ssh-agent` or aliases in `~/.ssh/config`.
4. **Context Guard**: Smart head/tail auto-truncation prevents token overflows.
5. **Git Safety-Net**: Automatic pre/post Git snapshots allow 1-click rollbacks if a change goes wrong.

---

## 🏗️ Architecture & How It Works

```
┌──────────────────────────────────────────────────────────────┐
│  AI Workbench (Claude / Cursor / Antigravity)  OR  Terminal  │
└──────────────────────────────┬───────────────────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            │  ai-ssh-tools (Dual-Mode Engine)    │
            │  • CLI Subcommand Runner            │
            │  • MCP Stdio Server (JSON-RPC)      │
            └──────────────────┬──────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            │       Resolution & Authentication   │
            │  1. Check ~/.ssh/config & profiles  │
            │  2. Query ssh-agent / local keys    │
            │  3. Verify TOFU host key fingerprint│
            └──────────────────┬──────────────────┘
                               │
            ┌──────────────────┴──────────────────┐
            │       Execution & Safety Layer      │
            │  • Command Policy Firewall          │
            │  • Smart Output Truncation (40KB)   │
            │  • Non-Interactive Sudo & PTY       │
            │  • Git Snapshot Wrap & Rollback     │
            │  • In-Memory Connection Pooling     │
            └──────────────────┬──────────────────┘
                               │ SSH / SFTP
                               ▼
            ┌─────────────────────────────────────┐
            │        Remote Target Server         │
            │    (Ubuntu, Debian, RHEL, etc.)     │
            └─────────────────────────────────────┘
```

---

## ⚡ CLI Quick Start

You do **not** need to create profiles or edit configuration files to get started. You can connect to any host on the fly:

### 1. Execute Remote Commands
```bash
# Direct connection with dynamic host and user
ai-ssh-tools exec --host 192.168.1.50 --user deploy "uptime"

# Run privileged command with sudo and PTY
ai-ssh-tools exec --host 192.168.1.50 --user deploy --sudo "systemctl restart nginx"

# Run inside a working directory with Git auto-rollback snapshot
ai-ssh-tools exec --host 192.168.1.50 --user deploy --workdir /opt/app --git "npm run build"
```

### 2. Check Real-Time System Vitals
```bash
# Human-readable summary (OS, Uptime, Load Avg, RAM, Disk mounts)
ai-ssh-tools vitals --host 192.168.1.50 --user deploy

# Output as structured JSON for automation scripts
ai-ssh-tools vitals --host 192.168.1.50 --user deploy --json
```

### 3. Inspect Docker Containers
```bash
# List running Docker containers
ai-ssh-tools docker --host 192.168.1.50 --user deploy

# Include stopped containers (equivalent to docker ps -a)
ai-ssh-tools docker --host 192.168.1.50 --user deploy --all
```

### 4. Manage `systemd` Services
```bash
# Check service status
ai-ssh-tools service --host 192.168.1.50 --user deploy --name nginx --action status

# Restart service (with sudo)
ai-ssh-tools service --host 192.168.1.50 --user deploy --name docker --action restart --sudo

# Tail last 100 lines of service logs from journalctl
ai-ssh-tools service --host 192.168.1.50 --user deploy --name nginx --action logs --lines 100
```

### 5. Tail Remote Log Files
```bash
ai-ssh-tools tail --host 192.168.1.50 --user deploy --path /var/log/nginx/access.log --lines 50
```

### 6. High-Throughput SFTP File Streaming
```bash
# Upload local file to remote server
ai-ssh-tools transfer --host 192.168.1.50 --user deploy --src ./build.tar.gz --dst /opt/app/build.tar.gz --op upload

# Download remote file to local machine
ai-ssh-tools transfer --host 192.168.1.50 --user deploy --src /var/log/syslog --dst ./syslog.log --op download
```

---

## 📦 Installation Guide (All Operating Systems)

### Option 1: Quick Install via `go install` (Recommended if Go is installed)
```bash
go install github.com/khalidelmerrah/ai-ssh-tools/cmd/ai-ssh-tools@latest
```

### Option 2: Pre-compiled Binaries

#### 🍎 macOS (Apple Silicon M1/M2/M3 & Intel)
```bash
# For Apple Silicon (arm64):
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-darwin-arm64
chmod +x ai-ssh-tools
sudo mv ai-ssh-tools /usr/local/bin/

# For Intel (amd64):
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-darwin-amd64
chmod +x ai-ssh-tools
sudo mv ai-ssh-tools /usr/local/bin/
```

#### 🐧 Linux (x86_64 & ARM64)
```bash
# For x86_64 (amd64):
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-linux-amd64
chmod +x ai-ssh-tools
sudo mv ai-ssh-tools /usr/local/bin/

# For ARM64 (Raspberry Pi, AWS Graviton, etc.):
curl -fsSL -o ai-ssh-tools https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-linux-arm64
chmod +x ai-ssh-tools
sudo mv ai-ssh-tools /usr/local/bin/
```

#### 🪟 Windows (PowerShell)
```powershell
# Create installation folder in LocalAppData
$InstallDir = "$env:LOCALAPPDATA\ai-ssh-tools"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# Download Windows binary
Invoke-WebRequest -Uri "https://github.com/khalidelmerrah/ai-ssh-tools/releases/latest/download/ai-ssh-tools-windows-amd64.exe" -OutFile "$InstallDir\ai-ssh-tools.exe"

# Add to user PATH (if not already added)
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    $env:Path += ";$InstallDir"
    Write-Host "[✓] Added ai-ssh-tools to User PATH" -ForegroundColor Green
}
```

---

## 🤖 Agent Setup & Configuration (All Workbenches)

`ai-ssh-tools` runs as a standard Model Context Protocol (MCP) server over `stdio`. Configure your preferred AI client below:

### 1. 🟣 Claude Desktop

Edit your configuration file:
* **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
* **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
* **Linux**: `~/.config/Claude/claude_desktop_config.json`

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
*(If `ai-ssh-tools` is not in your global PATH, use the full absolute path, e.g. `C:\\Users\\yourname\\AppData\\Local\\ai-ssh-tools\\ai-ssh-tools.exe` or `/usr/local/bin/ai-ssh-tools`).*

---

### 2. ⚡ Cursor IDE

1. Open **Cursor Settings** (`Ctrl + ,` or `Cmd + ,`).
2. Navigate to **Features** > **MCP Servers**.
3. Click **+ Add New MCP Server**.
4. Configure:
   * **Name**: `ai-ssh-tools`
   * **Type**: `command`
   * **Command**: `ai-ssh-tools serve`

---

### 3. 🌀 Antigravity / Gemini CLI

Add to your project root `antigravity.json` or global `~/.gemini/antigravity/mcp_servers.json`:

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

---

### 4. 🌊 Windsurf Editor

Edit `~/.codeium/windsurf/mcp_config.json`:

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

---

### 5. 🤖 Cline / Roo Code (VS Code Extension)

In VS Code, click the **MCP Servers** icon in the Cline panel (or edit `mcp_settings.json`):

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

---

### 6. 🔄 Continue.dev (VS Code & JetBrains)

Edit `~/.continue/config.json`:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "stdio",
          "command": "ai-ssh-tools",
          "args": ["serve"]
        }
      }
    ]
  }
}
```

---

### 7. ⚡ Zed Editor

Edit `~/.config/zed/settings.json`:

```json
{
  "context_servers": {
    "ai-ssh-tools": {
      "command": "ai-ssh-tools",
      "args": ["serve"]
    }
  }
}
```

---

## 🧠 System Prompt / Agent Rules (Recommended)

To ensure your AI assistant gets the most out of `ai-ssh-tools`, copy and paste these guidelines into your agent's **Custom Instructions**, `.cursorrules`, or `CLAUDE.md`:

```markdown
### SSH & Remote Server Operations Guidelines:
- You have access to remote servers via the `ai-ssh-tools` MCP server or CLI.
- Connect dynamically using `host` (IP/domain) and `user` (e.g. root, ubuntu, deploy). You do NOT need pre-saved profiles.
- Never output or request raw SSH private keys or passwords in the chat. Authentication is resolved automatically on the local machine via ssh-agent and SSH keys.
- Always execute single, atomic commands per tool call. Do not chain commands with `;`, `&&`, `||`, or backticks.
- When performing changes, deployments, or modifying files in remote git repositories, set `git_wrapped: true` with a `workdir` to allow instant rollback via `git_rollback` if errors occur.
- For system health, use `get_system_vitals` or `docker_containers` to receive clean, structured JSON metrics.
- For privileged commands, use `sudo: true` (or `--sudo`).
- For long-running commands (builds, daemon starts), use `manage_remote_process` to track background execution via task tokens.
```

---

## 🛠️ MCP Tools Reference

`ai-ssh-tools` exposes **12 purpose-built MCP tools**:

| Tool | Purpose | Key Parameters |
|---|---|---|
| `connect_and_execute` | Execute single atomic shell command with optional Git snapshot wrap | `host`, `user`, `command`, `workdir`, `git_wrapped`, `sudo`, `pty`, `timeout_seconds` |
| `get_system_vitals` | Return structured OS name, uptime, load averages, memory, and disk mounts in JSON | `host`, `user`, `port`, `profile` |
| `docker_containers` | Inspect running and stopped Docker containers (ID, Image, Status, Names) | `host`, `user`, `all`, `profile` |
| `manage_service` | Manage systemd units (`status`, `start`, `stop`, `restart`, `reload`, `logs`) | `host`, `user`, `name`, `action`, `lines`, `sudo` |
| `tail_remote_file` | Tail trailing lines from any remote log or text file | `host`, `user`, `path`, `lines` |
| `secure_file_delta` | Read (capped at 128KB), write, or list files via SFTP | `host`, `user`, `operation` (`read`/`write`/`list`), `path`, `content` |
| `secure_file_transfer` | Stream large files and binaries via SFTP buffer | `host`, `user`, `local_path`, `remote_path`, `direction` (`upload`/`download`) |
| `git_rollback` | Undo recent changes wrapped by automatic agent Git snapshots | `host`, `user`, `workdir`, `commits_back` |
| `manage_remote_process`| Supervise long-running background tasks (`start`, `status`, `logs`, `stop`) | `host`, `user`, `action`, `command`, `process_id` |
| `ssh_port_forward` | Open/close local SSH port forwarding tunnels | `host`, `user`, `action` (`start`/`stop`/`list`), `local_port`, `remote_port` |
| `list_profiles` | List loaded SSH connection profiles and metadata (excluding secrets) | *(none)* |
| `save_ssh_profile` | Dynamically create or update a profile in `ssh_hosts.json` | `alias`, `host`, `user`, `key_path`, `readonly`, etc. |

---

## 🔒 Security, Safety & Isolation

### 1. TOFU (Trust On First Use) Host Key Verification
Host identity validation is strictly enforced:
* When connecting to a host for the first time without an explicit `host_key`, `ai-ssh-tools` records the SHA256 fingerprint in `~/.ai-ssh-tools/known_hosts.json`.
* Subsequent connections verify the server's public key against this fingerprint. If a key changes unexpectedly, connections are aborted to protect against Man-in-the-Middle (MITM) attacks.

### 2. Context Window & Token Overflow Guard
If a remote command produces tens of thousands of lines of output, sending the full output to an LLM will exhaust token context or crash the session.
* Output exceeding **40 KB** or **400 lines** is automatically truncated.
* Preserves the **first 30 lines** (head) and **last 100 lines** (tail) along with an explicit indicator: `... [TRUNCATED 3,500 lines / 95,000 bytes] ...`.

### 3. Command Injection & Chaining Firewall
`connect_and_execute` blocks shell chaining operators (`;`, `&&`, `||`, backticks, `$()`). The AI must issue single atomic commands per turn, preventing concatenated prompt-injection payloads.

### 4. Automated Git Snapshot Checkpoints
Setting `git_wrapped: true` with a `workdir`:
1. Creates an automatic Git commit snapshot prior to command execution.
2. Executes the command.
3. Creates a post-execution auto-save snapshot.
4. If a build or deployment breaks, calling `git_rollback` instantly restores the working directory.

### 5. Read-Only Profiles & Path Boundaries
* **Read-Only Mode**: Profiles marked `"readonly": true` block all command execution, file modifications, service restarts, and process starts.
* **Path Whitelisting**: `"allowed_paths"` restricts SFTP operations to designated safe directory trees, preventing traversal attacks (e.g. `/var/log/../../etc/shadow`).

### 6. Append-Only Audit Logging
Every operation is logged in structured JSON to `~/.ai-ssh-tools/audit.log` (recording timestamp, user, host, command, exit code, and execution duration). Secrets and sensitive payloads are never written to disk.

---

## ⚙️ Profile Configuration (`ssh_hosts.json`)

While profiles are optional, creating named profiles allows you to set custom security boundaries, rate limits, and command policies:

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
  },
  {
    "alias":            "staging-db",
    "host":             "10.0.1.5",
    "port":             2222,
    "user":             "readonly_user",
    "use_agent":        true,
    "readonly":         true
  }
]
```

---

## 🔨 Building from Source

### Prerequisites
* [Go 1.22+](https://golang.org/dl/)

### Build Commands
```bash
# Build local optimized binary
go build -ldflags="-s -w" -o ai-ssh-tools ./cmd/ai-ssh-tools

# Run full test suite
go test -v ./...

# Cross-compile for Linux, macOS, Windows (amd64 + arm64)
./build.sh          # On Linux/macOS
pwsh .\build.ps1    # On Windows
```

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
