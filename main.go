// ai-ssh-tools: A production-ready MCP server for AI-driven SSH operations.
// Build: go build -ldflags="-s -w" -o ai-ssh-tools .
package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Redirect internal logs to stderr so they do not corrupt the MCP stdio stream.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[ai-ssh-tools] ")

	// Verify SSH library is available by attempting a no-op TCP dial timeout test.
	_ = func() { _, _ = net.DialTimeout("tcp", "127.0.0.1:1", time.Millisecond) }

	// Load host profiles from disk.
	if err := loadProfiles(); err != nil {
		log.Printf("[error] %v", err)
	}

	// ── Build MCP server ────────────────────────────────────────────────────
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ai-ssh-tools",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		Instructions: `You are connected to the ai-ssh-tools MCP server.

KEY RULES:
- Never ask for or repeat SSH credentials, private key content, or passwords.
- Execute only one atomic command per tool call (no chaining with ; && || or backticks).
- Use the 'profile' field where possible to avoid storing host/user in the conversation.
- When modifying remote files, prefer setting git_wrapped=true and a workdir to enable rollback.
- For diagnostics, use the /diagnose prompt. For logs, use the /logs prompt.`,
	})

	// ── Register tools ──────────────────────────────────────────────────────
	mcp.AddTool(server, &mcp.Tool{
		Name: "connect_and_execute",
		Description: `Connect to a remote SSH server and execute a single, non-interactive command.

Credentials are resolved server-side from named profiles or local SSH keys — they are NEVER exposed to the AI context.

SECURITY: Commands containing shell-chaining operators (; && || \` + "`" + `) are blocked. 
Submit only single, atomic commands. For multi-step operations, make multiple sequential tool calls.

Supports optional git snapshot wrapping (set git_wrapped=true with a workdir) to create automatic pre/post commit checkpoints for safe rollback.`,
	}, handleConnectAndExecute)

	mcp.AddTool(server, &mcp.Tool{
		Name: "secure_file_delta",
		Description: `Read, write, or list remote files via SFTP over an existing SSH connection.

Operations:
- read:  Download up to max_bytes of a file's content (default 128 KB cap)
- write: Upload UTF-8 string content to a remote path (creates parent dirs automatically)
- list:  List directory contents with permissions, sizes, and file names

The connection is reused from the session pool — no extra SSH handshake overhead.`,
	}, handleSecureFileDelta)

	mcp.AddTool(server, &mcp.Tool{
		Name: "git_rollback",
		Description: `Roll back the remote git repository to undo the last agent's automatic snapshots.
Requires workdir path. Checks history safety before performing hard reset.`,
	}, handleGitRollback)

	mcp.AddTool(server, &mcp.Tool{
		Name: "ssh_port_forward",
		Description: `Manage SSH local port forwarding tunnels.
Allows starting or stopping an SSH tunnel from the client machine to the remote system/network.`,
	}, handleSshPortForward)

	mcp.AddTool(server, &mcp.Tool{
		Name: "secure_file_transfer",
		Description: `Upload or download binary and large files via SFTP stream.
Direction can be 'upload' (local to remote) or 'download' (remote to local).`,
	}, handleSecureFileTransfer)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_system_vitals",
		Description: `Fetch remote system health metrics (OS name, uptime, load averages, memory usage, and disk mounts) in structured JSON format.`,
	}, handleGetSystemVitals)

	mcp.AddTool(server, &mcp.Tool{
		Name: "manage_remote_process",
		Description: `Manage background processes executing on the remote server under isolation.
Actions:
- start: Start a command in the background (returns process_id).
- status: Check if running/completed and read exit code.
- logs: Tail stderr/stdout logs of the task.
- stop: Kill the background process securely.`,
	}, handleManageRemoteProcess)

	// ── Register prompts ─────────────────────────────────────────────────────
	server.AddPrompt(
		&mcp.Prompt{
			Name:        "diagnose",
			Description: "Run a sequential server vitals check (df -h, free -m, ss -tulpn, ps aux)",
			Arguments: []*mcp.PromptArgument{
				{Name: "profile", Description: "Named SSH profile alias from ssh_hosts.json", Required: false},
				{Name: "host", Description: "Remote hostname (used when profile is not set)", Required: false},
			},
		},
		handleDiagnosePrompt,
	)
	server.AddPrompt(
		&mcp.Prompt{
			Name:        "logs",
			Description: "Fetch and analyse the last N lines of a service's logs",
			Arguments: []*mcp.PromptArgument{
				{Name: "service", Description: "Systemd unit name or docker container name", Required: true},
				{Name: "profile", Description: "Named SSH profile alias", Required: false},
				{Name: "host", Description: "Remote hostname", Required: false},
				{Name: "lines", Description: "Number of log lines to fetch (default 100)", Required: false},
			},
		},
		handleLogsPrompt,
	)
	server.AddPrompt(
		&mcp.Prompt{
			Name:        "deploy",
			Description: "Execute an atomic deployment pipeline on a remote host",
			Arguments: []*mcp.PromptArgument{
				{Name: "workdir", Description: "Absolute path to the project root on the remote host", Required: true},
				{Name: "profile", Description: "Named SSH profile alias", Required: false},
				{Name: "host", Description: "Remote hostname", Required: false},
				{Name: "branch", Description: "Git branch to deploy (default: main)", Required: false},
				{Name: "service", Description: "Systemd service to restart after deploy", Required: false},
				{Name: "pm2_name", Description: "PM2 process name to restart", Required: false},
				{Name: "migrate", Description: "Run DB migrations (true/false)", Required: false},
			},
		},
		handleDeployPrompt,
	)

	// ── Start stdio transport ────────────────────────────────────────────────
	log.Println("ai-ssh-tools MCP server starting on stdio transport")

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
