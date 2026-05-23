# ai-ssh-tools — MCP SSH Agent for AI Workbenches

A production-grade, modular, standalone **Model Context Protocol (MCP) server** written in Go. It gives AI agents (Claude, Gemini, etc.) a secure, zero-latency SSH client with connection pooling, command sanitisation, SFTP file operations, local port forwarding (tunneling), system vitals diagnostic reporting, and background process supervision with an automated Git safety-net.

---

## ✨ Features

| Capability | Detail |
|---|---|
| **Connection pooling** | Open SSH connections cached in-memory (`user@host:port`) — no re-handshake per turn. |
| **Keepalive** | 30-second background ping prevents stale sessions. |
| **Seven MCP Tools** | `connect_and_execute`, `secure_file_delta`, `git_rollback`, `ssh_port_forward`, `secure_file_transfer`, `get_system_vitals`, `manage_remote_process`. |
| **Three MCP Prompts** | `/diagnose`, `/logs`, `/deploy`. |
| **Git Safety-net** | Pre/post `git commit` snapshots around executions for quick safety rollback. |
| **Command Firewall** | Enforces whitelist (`allowed_commands`), blacklist (`blocked_commands`), and blocks chaining operators (`;`, `&&`, `||`, backticks, `$()`). |
| **Strict Host Key** | Validates connection fingerprints (`HostKey` check) via SHA256, MD5, or base64. |
| **Credential Masking** | Private keys & passwords resolved server-side, never leaked to the LLM context. |
| **SFTP transfers** | High-throughput streaming transfers (`secure_file_transfer`) for large files / binaries. |
| **Background Processes** | Spawns background tasks (`nohup` wrapper) on the remote host, monitoring status and logs via task tokens. |

---

## 🚀 Quick Start

### 1. Installation

Download the precompiled binary for your system from the **GitHub Releases** page, or build it locally:

```bash
# Clone the repository
git clone https://github.com/khalidelmerrah/ai-ssh-tools
cd ai-ssh-tools

# Build optimized local binary (stripped symbols + DWARF)
go build -ldflags="-s -w" -o ai-ssh-tools .

# Or run cross-platform build script
./build.sh
```

### 2. Configure Your Hosts

Copy and edit `ssh_hosts.json` next to your compiled binary:

```json
[
  {
    "alias":            "prod-web",
    "host":             "203.0.113.10",
    "port":             22,
    "user":             "deploy",
    "key_path":         "~/.ssh/id_ed25519",
    "host_key":         "SHA256:abc123xyz...", 
    "allowed_commands": ["^git.*$", "^systemctl status nginx$"],
    "blocked_commands": [".*rm -rf.*"],
    "git_enabled":      true
  }
]
```

* **`host_key`**: Optional. Matches SHA256/MD5 public key fingerprint or raw base64 string to verify identity.
* **`allowed_commands`**: Optional regex whitelist. If set, only matching commands are allowed.
* **`blocked_commands`**: Optional regex blacklist. If matching, command is blocked.

### 3. Register with your AI Workbench

Add to your MCP server configuration file (e.g. `mcp_servers.json`):

```json
{
  "mcpServers": {
    "ai-ssh-tools": {
      "command": "/absolute/path/to/ai-ssh-tools",
      "args": [],
      "env": {
        "SSH_HOSTS_PATH": "/absolute/path/to/ssh_hosts.json"
      }
    }
  }
}
```

---

## 🔧 Tools Reference

### `connect_and_execute`
Execute a single remote command.
* **`profile`** (string, optional): Named profile from config.
* **`host`** / **`user`** / **`port`** (optional): Ad-hoc connection parameters.
* **`command`** (string, required): Command to execute. No chaining allowed.
* **`workdir`** (string, optional): Working directory.
* **`git_wrapped`** (bool, optional): Wrap execution in git commits.

### `secure_file_delta`
Perform SFTP operations (`read`, `write`, `list`). Cap of 128 KB applied to `read` to avoid LLM context flood.

### `git_rollback`
Roll back changes inside a git-wrapped repository.
* **`workdir`** (string, required): Git repository root.
* **`commits_back`** (int, default 2): Number of commits to roll back.

### `ssh_port_forward`
Manage local-to-remote SSH tunnels.
* **`action`** (string, required): `start`, `stop`, or `list`.
* **`local_port`** (int): Port to bind on the local machine.
* **`remote_host`** / **`remote_port`**: Destination inside the remote network.

### `secure_file_transfer`
Upload or download large files and binaries using streaming SFTP buffers.
* **`direction`** (string, required): `upload` or `download`.
* **`local_path`** / **`remote_path`** (string, required): File paths.

### `get_system_vitals`
Returns CPU load averages, memory status, and disk space usage in structured JSON.

### `manage_remote_process`
Supervise background processes under `~/.ai_ssh_processes/` on the remote host.
* **`action`** (string, required): `start`, `status`, `logs`, or `stop`.
* **`command`** (string): Shell command to execute (required for `start`).
* **`process_id`** (string): Unique identifier.
* **`lines`** (int): Number of log lines to fetch.

---

## 🤖 Instructions for AI Agents

> [!IMPORTANT]
> **Read these guidelines carefully when operating this MCP server.**

### 1. Prefer Profiles and Aliases
* **Do not request or output credentials.** Check `ssh_hosts.json` profiles first. Always prefer using the `"profile"` parameter instead of explicit `"host"` and `"user"` arguments.

### 2. Follow Command Safety Policies
* Command chaining is strictly blocked by the firewall. Do not attempt to run multiple commands separated by `;`, `&&`, `||`, or backticks in `connect_and_execute`. Instead, execute them in **individual, sequential tool calls**.

### 3. Deploying & Long-Running Builds
* For commands that take more than 5 seconds (such as starting a dev server, running extensive builds, or compiling packages), **do not use `connect_and_execute`** (which will block and might timeout).
* Instead, run them using **`manage_remote_process`** with `action: "start"`. 
* Retrieve the `process_id` and poll `action: "status"` and `action: "logs"` sequentially until the task completes.

### 4. Git Rolled Backups
* When making file edits in a git repository on the remote server, set `git_wrapped: true` and specify the `workdir` in `connect_and_execute`.
* If a change causes a regression or a compilation failure, immediately call **`git_rollback`** on the repository root to discard the changes safely.

### 5. Large File Handling
* To read configurations or check file snippets, use `secure_file_delta`.
* To upload binaries, libraries, or transfer zip files, use **`secure_file_transfer`** to stream the binary bytes directly from/to your environment, rather than encoding file content inside text-based RPC formats.
