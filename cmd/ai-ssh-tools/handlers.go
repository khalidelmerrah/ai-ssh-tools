package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ─────────────────────────────────────────────────────────────────────────────
// TOOL INPUT STRUCTS
// ─────────────────────────────────────────────────────────────────────────────

// ConnectAndExecuteArgs are the typed inputs for the connect_and_execute tool.
type ConnectAndExecuteArgs struct {
	Profile        string `json:"profile,omitempty" jsonschema:"Named profile alias from ssh_hosts.json (mutually exclusive with host/user)"`
	Host           string `json:"host,omitempty" jsonschema:"Remote hostname or IP (used when profile is not specified)"`
	User           string `json:"user,omitempty" jsonschema:"SSH username (used when profile is not specified)"`
	Port           int    `json:"port,omitempty" jsonschema:"SSH port — defaults to 22 (used when profile is not specified)"`
	Command        string `json:"command" jsonschema:"Shell command to execute on the remote host. Must be a single atomic command with no shell chaining operators."`
	Workdir        string `json:"workdir,omitempty"     jsonschema:"Absolute path on the remote host to use as the working directory. When set, the command is wrapped in pre/post git snapshots for rollback safety."`
	GitWrapped     bool   `json:"git_wrapped,omitempty" jsonschema:"When true and workdir is set, wraps execution in pre/post git snapshots (requires git on the remote)."`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty" jsonschema:"Timeout in seconds for command execution (default 30, max 300)"`
	Sudo           bool   `json:"sudo,omitempty" jsonschema:"Execute command with sudo privileges"`
	SudoPassword   string `json:"sudo_password,omitempty" jsonschema:"Sudo password if required (fed non-interactively via stdin, never logged)"`
	Pty            bool   `json:"pty,omitempty" jsonschema:"Allocate a pseudo-terminal (PTY) for the session"`
}

// SecureFileDeltaArgs are the typed inputs for the secure_file_delta tool.
type SecureFileDeltaArgs struct {
	Profile    string `json:"profile,omitempty"      jsonschema:"Named profile alias from ssh_hosts.json"`
	Host       string `json:"host,omitempty"         jsonschema:"Remote hostname or IP"`
	User       string `json:"user,omitempty"         jsonschema:"SSH username"`
	Port       int    `json:"port,omitempty"         jsonschema:"SSH port (default 22)"`
	Operation  string `json:"operation"              jsonschema:"File operation: read (download content), write (upload content), list (list directory)"`
	RemotePath string `json:"remote_path"            jsonschema:"Absolute path on the remote host"`
	Content    string `json:"content,omitempty"      jsonschema:"UTF-8 file content to write (only for write operation)"`
	MaxBytes   int64  `json:"max_bytes,omitempty"    jsonschema:"Maximum bytes to read (default 131072 / 128 KB to keep LLM context manageable)"`
}

// GitRollbackArgs are the typed inputs for the git_rollback tool.
type GitRollbackArgs struct {
	Profile     string `json:"profile,omitempty"      jsonschema:"Named profile alias from ssh_hosts.json"`
	Host        string `json:"host,omitempty"         jsonschema:"Remote hostname or IP"`
	User        string `json:"user,omitempty"         jsonschema:"SSH username"`
	Port        int    `json:"port,omitempty"         jsonschema:"SSH port (default 22)"`
	Workdir     string `json:"workdir"                 jsonschema:"Absolute path on the remote host to the git repository root"`
	CommitsBack int    `json:"commits_back,omitempty" jsonschema:"Number of commits to roll back (default 2, which undoes the pre and post changes of the last agent execution)"`
	Force       bool   `json:"force,omitempty"        jsonschema:"Force rollback even if safety checks fail"`
}

// SshPortForwardArgs are the typed inputs for the ssh_port_forward tool.
type SshPortForwardArgs struct {
	Profile    string `json:"profile,omitempty"   jsonschema:"Named profile alias from ssh_hosts.json"`
	Host       string `json:"host,omitempty"      jsonschema:"Remote hostname or IP"`
	User       string `json:"user,omitempty"      jsonschema:"SSH username"`
	Port       int    `json:"port,omitempty"      jsonschema:"SSH port (default 22)"`
	Action     string `json:"action"              jsonschema:"Action: start, stop, or list tunnels"`
	LocalPort  int    `json:"local_port,omitempty" jsonschema:"Local port to bind on client machine"`
	RemoteHost string `json:"remote_host,omitempty" jsonschema:"Target host reachable from the remote SSH server (default: localhost)"`
	RemotePort int    `json:"remote_port,omitempty" jsonschema:"Target port on the remote host"`
}

// SecureFileTransferArgs are the typed inputs for the secure_file_transfer tool.
type SecureFileTransferArgs struct {
	Profile    string `json:"profile,omitempty"   jsonschema:"Named profile alias from ssh_hosts.json"`
	Host       string `json:"host,omitempty"      jsonschema:"Remote hostname or IP"`
	User       string `json:"user,omitempty"      jsonschema:"SSH username"`
	Port       int    `json:"port,omitempty"      jsonschema:"SSH port (default 22)"`
	Direction  string `json:"direction"           jsonschema:"Transfer direction: upload or download"`
	LocalPath  string `json:"local_path"          jsonschema:"Absolute local file path on the client machine"`
	RemotePath string `json:"remote_path"         jsonschema:"Absolute remote file path on the server"`
}

// GetSystemVitalsArgs are the typed inputs for the get_system_vitals tool.
type GetSystemVitalsArgs struct {
	Profile string `json:"profile,omitempty" jsonschema:"Named profile alias from ssh_hosts.json"`
	Host    string `json:"host,omitempty"    jsonschema:"Remote hostname or IP"`
	User    string `json:"user,omitempty"    jsonschema:"SSH username"`
	Port    int    `json:"port,omitempty"    jsonschema:"SSH port (default 22)"`
}

// ManageRemoteProcessArgs are the typed inputs for the manage_remote_process tool.
type ManageRemoteProcessArgs struct {
	Profile   string `json:"profile,omitempty"   jsonschema:"Named profile alias from ssh_hosts.json"`
	Host      string `json:"host,omitempty"      jsonschema:"Remote hostname or IP"`
	User      string `json:"user,omitempty"      jsonschema:"SSH username"`
	Port      int    `json:"port,omitempty"      jsonschema:"SSH port (default 22)"`
	Action    string `json:"action"              jsonschema:"Process action: start (run in background), status (check execution status), stop (kill process), logs (read stdout/stderr logs)"`
	Command   string `json:"command,omitempty"   jsonschema:"The command to execute (required only for start)"`
	ProcessID string `json:"process_id,omitempty" jsonschema:"Unique identifier of the process (required for status, stop, logs)"`
	Lines     int    `json:"lines,omitempty"      jsonschema:"Number of tail lines of logs to retrieve (default 100)"`
	Workdir   string `json:"workdir,omitempty"    jsonschema:"Working directory on the remote server to start the process in"`
}

// ListProfilesArgs has no arguments.
type ListProfilesArgs struct{}

// SaveSshProfileArgs are the typed inputs for the save_ssh_profile tool.
type SaveSshProfileArgs struct {
	Alias           string   `json:"alias" jsonschema:"The user-friendly name of the profile"`
	Host            string   `json:"host" jsonschema:"Remote hostname or IP"`
	Port            int      `json:"port,omitempty" jsonschema:"SSH port (default 22)"`
	User            string   `json:"user" jsonschema:"SSH username"`
	KeyPath         string   `json:"key_path,omitempty" jsonschema:"Absolute path to private key on the client machine"`
	Password        string   `json:"password,omitempty" jsonschema:"SSH password (optional)"`
	UseAgent        bool     `json:"use_agent,omitempty" jsonschema:"Whether to use the SSH agent socket (optional)"`
	GitEnabled      bool     `json:"git_enabled,omitempty" jsonschema:"Wrap executions in pre/post git snapshots (optional)"`
	AllowedCommands []string `json:"allowed_commands,omitempty" jsonschema:"List of allowed regexes for commands (optional)"`
	BlockedCommands []string `json:"blocked_commands,omitempty" jsonschema:"List of blocked regexes for commands (optional)"`
	HostKey         string   `json:"host_key,omitempty" jsonschema:"Expected host key or fingerprint (optional)"`
	ReadOnly        bool     `json:"readonly,omitempty" jsonschema:"Configure the profile as read-only (optional)"`
	RateLimitRPM    *int     `json:"rate_limit_rpm,omitempty" jsonschema:"Custom rate limit in RPM (optional)"`
	AllowedPaths    []string `json:"allowed_paths,omitempty" jsonschema:"List of allowed SFTP paths (optional)"`
}

// DockerContainersArgs are the typed inputs for docker_containers.
type DockerContainersArgs struct {
	Profile string `json:"profile,omitempty" jsonschema:"Named profile alias from ssh_hosts.json or ~/.ssh/config"`
	Host    string `json:"host,omitempty"    jsonschema:"Remote hostname or IP"`
	User    string `json:"user,omitempty"    jsonschema:"SSH username"`
	Port    int    `json:"port,omitempty"    jsonschema:"SSH port (default 22)"`
	All     bool   `json:"all,omitempty"     jsonschema:"Include stopped containers (equivalent to docker ps -a)"`
}

// ManageServiceArgs are the typed inputs for manage_service.
type ManageServiceArgs struct {
	Profile      string `json:"profile,omitempty"       jsonschema:"Named profile alias from ssh_hosts.json or ~/.ssh/config"`
	Host         string `json:"host,omitempty"          jsonschema:"Remote hostname or IP"`
	User         string `json:"user,omitempty"          jsonschema:"SSH username"`
	Port         int    `json:"port,omitempty"          jsonschema:"SSH port (default 22)"`
	Name         string `json:"name"                    jsonschema:"Service name (e.g. nginx, docker, postgresql)"`
	Action       string `json:"action"                  jsonschema:"Action: status, start, stop, restart, enable, disable, logs"`
	Lines        int    `json:"lines,omitempty"         jsonschema:"Number of log lines when action is 'logs' (default 50)"`
	Sudo         bool   `json:"sudo,omitempty"          jsonschema:"Run with sudo privileges"`
	SudoPassword string `json:"sudo_password,omitempty" jsonschema:"Sudo password if needed"`
}

// TailRemoteFileArgs are the typed inputs for tail_remote_file.
type TailRemoteFileArgs struct {
	Profile string `json:"profile,omitempty" jsonschema:"Named profile alias from ssh_hosts.json or ~/.ssh/config"`
	Host    string `json:"host,omitempty"    jsonschema:"Remote hostname or IP"`
	User    string `json:"user,omitempty"    jsonschema:"SSH username"`
	Port    int    `json:"port,omitempty"    jsonschema:"SSH port (default 22)"`
	Path    string `json:"path"              jsonschema:"Absolute path to the remote log or file to tail"`
	Lines   int    `json:"lines,omitempty"   jsonschema:"Number of trailing lines to read (default 50, max 1000)"`
}

// ─────────────────────────────────────────────────────────────────────────────
// PROFILE RESOLUTION
// ─────────────────────────────────────────────────────────────────────────────

func resolveProfile(profileAlias, host, user string, port int) (*HostProfile, error) {
	if profileAlias != "" {
		profileRegistryMu.RLock()
		p, ok := profileRegistry[profileAlias]
		profileRegistryMu.RUnlock()
		if ok {
			return p, nil
		}
		// Check ~/.ssh/config fallback for alias
		if cfg := resolveSSHConfig(profileAlias); cfg != nil {
			targetHost := cfg.HostName
			if targetHost == "" {
				targetHost = profileAlias
			}
			targetUser := cfg.User
			if targetUser == "" {
				targetUser = user
			}
			targetPort := cfg.Port
			if targetPort == 0 {
				targetPort = 22
			}
			if targetUser != "" {
				return &HostProfile{
					Alias:   profileAlias,
					Host:    targetHost,
					Port:    targetPort,
					User:    targetUser,
					KeyPath: cfg.IdentityFile,
				}, nil
			}
		}
		return nil, fmt.Errorf("unknown profile %q — check ssh_hosts.json or ~/.ssh/config", profileAlias)
	}

	// Try resolving host from ~/.ssh/config if user is missing
	if host != "" && user == "" {
		if cfg := resolveSSHConfig(host); cfg != nil && cfg.User != "" {
			targetHost := cfg.HostName
			if targetHost == "" {
				targetHost = host
			}
			targetPort := cfg.Port
			if port != 0 {
				targetPort = port
			} else if targetPort == 0 {
				targetPort = 22
			}
			return &HostProfile{
				Alias:   host,
				Host:    targetHost,
				Port:    targetPort,
				User:    cfg.User,
				KeyPath: cfg.IdentityFile,
			}, nil
		}
	}

	if host == "" || user == "" {
		return nil, fmt.Errorf("either 'profile' or both 'host' and 'user' must be provided")
	}
	if port == 0 {
		port = 22
	}
	return &HostProfile{
		Alias: host,
		Host:  host,
		Port:  port,
		User:  user,
	}, nil
}

func checkRateLimitForProfile(profile *HostProfile) error {
	key := poolKey(profile.User, profile.Host, profile.Port)
	return checkRateLimit(profile.Alias, key, profile.RateLimitRPM)
}

// ─────────────────────────────────────────────────────────────────────────────
// PATH SECURITY HELPER
// ─────────────────────────────────────────────────────────────────────────────

func isPathAllowed(allowedPaths []string, remotePath string) bool {
	if len(allowedPaths) == 0 {
		return true
	}
	cleaned := path.Clean(remotePath)
	for _, allowed := range allowedPaths {
		cleanedAllowed := path.Clean(allowed)
		if cleaned == cleanedAllowed {
			return true
		}
		prefix := cleanedAllowed
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if strings.HasPrefix(cleaned, prefix) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// EXECUTION HELPERS
// ─────────────────────────────────────────────────────────────────────────────

type execResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func remoteExec(ctx context.Context, client *ssh.Client, cmd string) (*execResult, error) {
	return remoteExecOpts(ctx, client, cmd, false, false, "")
}

func remoteExecOpts(ctx context.Context, client *ssh.Client, cmd string, pty bool, sudo bool, sudoPassword string) (*execResult, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new SSH session: %w", err)
	}
	defer sess.Close()

	if pty {
		modes := ssh.TerminalModes{
			ssh.ECHO:          0,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if err := sess.RequestPty("xterm-256color", 80, 24, modes); err != nil {
			log.Printf("[warn] request pty failed: %v", err)
		}
	} else {
		_ = sess.Setenv("DEBIAN_FRONTEND", "noninteractive")
		_ = sess.Setenv("TERM", "dumb")
	}

	var stdinWriter io.WriteCloser
	if sudo && sudoPassword != "" {
		stdinPipe, err := sess.StdinPipe()
		if err == nil {
			stdinWriter = stdinPipe
		}
		cmd = fmt.Sprintf("sudo -S -p '' %s", cmd)
	} else if sudo {
		cmd = fmt.Sprintf("sudo %s", cmd)
	}

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		if stdinWriter != nil {
			_, _ = io.WriteString(stdinWriter, sudoPassword+"\n")
			_ = stdinWriter.Close()
		}
		done <- sess.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		sess.Close()
		<-done
		return nil, fmt.Errorf("command execution timed out: %w", ctx.Err())
	case err := <-done:
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return nil, fmt.Errorf("run command: %w", err)
			}
		}

		return &execResult{
			Stdout:   smartTruncate(stdout.String(), 40*1024, 400),
			Stderr:   smartTruncate(stderr.String(), 40*1024, 400),
			ExitCode: exitCode,
		}, nil
	}
}

func gitWrappedExec(ctx context.Context, client *ssh.Client, workdir, cmd string, pty, sudo bool, sudoPassword string) (*execResult, error) {
	if _, err := remoteExec(ctx, client, fmt.Sprintf("cd %s && git init -q", shellQuote(workdir))); err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}

	_, _ = remoteExec(ctx, client, `git config --global user.email "ai-agent@localhost" 2>/dev/null || true`)
	_, _ = remoteExec(ctx, client, `git config --global user.name "AI Agent" 2>/dev/null || true`)

	preSnap := fmt.Sprintf(
		`cd %s && git add -A 2>/dev/null; git commit --allow-empty -m "Pre-agent snapshot" -q 2>/dev/null || true`,
		shellQuote(workdir),
	)
	if _, err := remoteExec(ctx, client, preSnap); err != nil {
		log.Printf("[warn] pre-snapshot failed: %v", err)
	}

	result, execErr := remoteExecOpts(ctx, client, fmt.Sprintf("cd %s && %s", shellQuote(workdir), cmd), pty, sudo, sudoPassword)

	sanitizedMsg := strings.ReplaceAll(cmd, `"`, `'`)
	if len(sanitizedMsg) > 72 {
		sanitizedMsg = sanitizedMsg[:72] + "..."
	}
	postSnap := fmt.Sprintf(
		`cd %s && git add -A 2>/dev/null; git commit --allow-empty -m "AI Auto-save: %s" -q 2>/dev/null || true`,
		shellQuote(workdir), sanitizedMsg,
	)
	if _, err := remoteExec(ctx, client, postSnap); err != nil {
		log.Printf("[warn] post-snapshot failed: %v", err)
	}

	return result, execErr
}

func formatExecResult(r *execResult) string {
	var sb strings.Builder
	if r.Stdout != "" {
		fmt.Fprintf(&sb, "STDOUT:\n%s\n", strings.TrimRight(r.Stdout, "\n"))
	}
	if r.Stderr != "" {
		fmt.Fprintf(&sb, "STDERR:\n%s\n", strings.TrimRight(r.Stderr, "\n"))
	}
	fmt.Fprintf(&sb, "Exit code: %d", r.ExitCode)
	return sb.String()
}

func textContent(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}
}

func errContent(format string, args ...any) *mcp.CallToolResult {
	msg := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SFTP OPERATIONS
// ─────────────────────────────────────────────────────────────────────────────

const defaultMaxBytes = 128 * 1024 // 128 KB cap

func sftpRead(c *sftp.Client, path string, maxBytes int64) (*mcp.CallToolResult, any, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	f, err := c.Open(path)
	if err != nil {
		return errContent("open %s: %v", path, err), nil, nil
	}
	defer f.Close()

	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	stat, _ := f.Stat()
	var note string
	if stat != nil && stat.Size() > maxBytes {
		note = fmt.Sprintf("\n\n[⚠ File truncated: showing first %d of %d bytes]", maxBytes, stat.Size())
	}
	return textContent(content + note), nil, nil
}

func sftpWrite(c *sftp.Client, path string, content string) (*mcp.CallToolResult, any, error) {
	dir := filepath.ToSlash(filepath.Dir(path))
	if err := c.MkdirAll(dir); err != nil {
		return errContent("mkdirall %s: %v", dir, err), nil, nil
	}

	f, err := c.Create(path)
	if err != nil {
		return errContent("create %s: %v", path, err), nil, nil
	}
	defer f.Close()

	n, err := f.Write([]byte(content))
	if err != nil {
		return errContent("write %s: %v", path, err), nil, nil
	}
	return textContent(fmt.Sprintf("✓ wrote %d bytes to %s", n, path)), nil, nil
}

func sftpList(c *sftp.Client, path string) (*mcp.CallToolResult, any, error) {
	entries, err := c.ReadDir(path)
	if err != nil {
		return errContent("readdir %s: %v", path, err), nil, nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Contents of %s (%d entries):\n\n", path, len(entries))
	for _, e := range entries {
		mode := e.Mode().String()
		size := e.Size()
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		fmt.Fprintf(&sb, "%s  %8d  %s\n", mode, size, name)
	}
	return textContent(sb.String()), nil, nil
}

func transferFileStream(client *ssh.Client, localPath, remotePath, direction string) (int64, error) {
	sfClient, err := sftp.NewClient(client)
	if err != nil {
		return 0, fmt.Errorf("SFTP session error: %w", err)
	}
	defer sfClient.Close()

	if direction == "upload" {
		localFile, err := os.Open(localPath)
		if err != nil {
			return 0, fmt.Errorf("failed to open local file %q: %w", localPath, err)
		}
		defer localFile.Close()

		dir := filepath.ToSlash(filepath.Dir(remotePath))
		if err := sfClient.MkdirAll(dir); err != nil {
			return 0, fmt.Errorf("mkdirall %s: %w", dir, err)
		}

		remoteFile, err := sfClient.Create(remotePath)
		if err != nil {
			return 0, fmt.Errorf("failed to create remote file %q: %w", remotePath, err)
		}
		defer remoteFile.Close()

		return io.Copy(remoteFile, localFile)
	} else if direction == "download" {
		remoteFile, err := sfClient.Open(remotePath)
		if err != nil {
			return 0, fmt.Errorf("failed to open remote file %q: %w", remotePath, err)
		}
		defer remoteFile.Close()

		dir := filepath.Dir(localPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return 0, fmt.Errorf("failed to create local directories %q: %w", dir, err)
		}

		localFile, err := os.Create(localPath)
		if err != nil {
			return 0, fmt.Errorf("failed to create local file %q: %w", localPath, err)
		}
		defer localFile.Close()

		return io.Copy(localFile, remoteFile)
	}

	return 0, fmt.Errorf("invalid direction %q (must be upload or download)", direction)
}

// ─────────────────────────────────────────────────────────────────────────────
// MCP TOOL HANDLERS
// ─────────────────────────────────────────────────────────────────────────────

func handleConnectAndExecute(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args ConnectAndExecuteArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	if profile.ReadOnly && args.GitWrapped {
		return errContent("profile is configured as read-only; git-wrapped write operations are not permitted"), nil, nil
	}

	cleanCmd, err := validateCommandPolicy(profile, args.Command)
	if err != nil {
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		return errContent("connection failed: %v", err), nil, nil
	}

	timeout := 30 * time.Second
	if args.TimeoutSeconds != nil {
		ts := *args.TimeoutSeconds
		if ts > 300 {
			ts = 300
		} else if ts < 1 {
			ts = 1
		}
		timeout = time.Duration(ts) * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var result *execResult
	if args.GitWrapped && args.Workdir != "" {
		result, err = gitWrappedExec(execCtx, client, args.Workdir, cleanCmd, args.Pty, args.Sudo, args.SudoPassword)
	} else {
		execCmd := cleanCmd
		if args.Workdir != "" {
			execCmd = fmt.Sprintf("cd %s && %s", shellQuote(args.Workdir), cleanCmd)
		}
		result, err = remoteExecOpts(execCtx, client, execCmd, args.Pty, args.Sudo, args.SudoPassword)
	}

	duration := time.Since(start).Milliseconds()

	var exitCode *int
	if result != nil {
		exitCode = &result.ExitCode
	}

	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}

	auditLog(AuditEntry{
		Profile:    profile.Alias,
		Host:       profile.Host,
		Tool:       "connect_and_execute",
		Command:    cleanCmd,
		ExitCode:   exitCode,
		DurationMs: duration,
		Error:      errMsg,
	})

	if err != nil {
		return errContent("execution error: %v", err), nil, nil
	}

	out := formatExecResult(result)
	return textContent(out), nil, nil
}

func handleSecureFileDelta(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args SecureFileDeltaArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	// 1. Enforce Allowed Paths
	if !isPathAllowed(profile.AllowedPaths, args.RemotePath) {
		err = fmt.Errorf("path %s is not within allowed paths for this profile", args.RemotePath)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_delta",
			Operation:  args.Operation,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	// 2. Enforce ReadOnly check
	if strings.ToLower(args.Operation) == "write" && profile.ReadOnly {
		err = fmt.Errorf("profile is configured as read-only; write operations are not permitted")
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_delta",
			Operation:  args.Operation,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_delta",
			Operation:  args.Operation,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("connection failed: %v", err), nil, nil
	}

	sfClient, err := sftp.NewClient(client)
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_delta",
			Operation:  args.Operation,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("SFTP session error: %v", err), nil, nil
	}
	defer sfClient.Close()

	var result *mcp.CallToolResult
	switch strings.ToLower(args.Operation) {
	case "read":
		result, _, err = sftpRead(sfClient, args.RemotePath, args.MaxBytes)
	case "write":
		if args.Content == "" {
			err = fmt.Errorf("'content' is required for the write operation")
			result = errContent("%v", err)
		} else {
			result, _, err = sftpWrite(sfClient, args.RemotePath, args.Content)
		}
	case "list":
		result, _, err = sftpList(sfClient, args.RemotePath)
	default:
		err = fmt.Errorf("unknown operation %q — must be read, write, or list", args.Operation)
		result = errContent("%v", err)
	}

	var errMsg string
	if err != nil {
		errMsg = err.Error()
	} else if result != nil && result.IsError {
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				errMsg = tc.Text
			}
		}
	}

	auditLog(AuditEntry{
		Profile:    profile.Alias,
		Host:       profile.Host,
		Tool:       "secure_file_delta",
		Operation:  args.Operation,
		Path:       args.RemotePath,
		DurationMs: time.Since(start).Milliseconds(),
		Error:      errMsg,
	})

	return result, nil, err
}

func handleGitRollback(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args GitRollbackArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	if profile.ReadOnly {
		err = fmt.Errorf("profile is configured as read-only; write operations are not permitted")
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("connection failed: %v", err), nil, nil
	}

	commitsBack := args.CommitsBack
	if commitsBack <= 0 {
		commitsBack = 2
	}

	res, err := remoteExec(ctx, client, fmt.Sprintf("cd %s && git rev-parse --is-inside-work-tree", shellQuote(args.Workdir)))
	if err != nil || res.ExitCode != 0 {
		var exitCode int
		if res != nil {
			exitCode = res.ExitCode
		}
		var stderr string
		if res != nil {
			stderr = res.Stderr
		}
		err = fmt.Errorf("directory %q is not a valid git repository: %v (exit code %d, stderr: %s)", args.Workdir, err, exitCode, stderr)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	logRes, err := remoteExec(ctx, client, fmt.Sprintf("cd %s && git log -n 1 --format=%%ae|%%ct", shellQuote(args.Workdir)))
	if err != nil || logRes.ExitCode != 0 {
		var exitCode int
		if logRes != nil {
			exitCode = logRes.ExitCode
		}
		var stderr string
		if logRes != nil {
			stderr = logRes.Stderr
		}
		err = fmt.Errorf("failed to read git log: %v (exit code %d, stderr: %s)", err, exitCode, stderr)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	output := strings.TrimSpace(logRes.Stdout)
	parts := strings.Split(output, "|")
	if len(parts) != 2 {
		err = fmt.Errorf("unexpected git log format: %q", output)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	authorEmail := parts[0]
	timestampStr := parts[1]

	commitTimestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		err = fmt.Errorf("failed to parse commit timestamp %q: %v", timestampStr, err)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	commitTime := time.Unix(commitTimestamp, 0)
	age := time.Since(commitTime)

	emailMatch := authorEmail == "ai-agent@localhost"
	ageOk := age < 24*time.Hour

	if !args.Force {
		if !emailMatch {
			err = fmt.Errorf("safety check failed: most recent commit author is %q, expected \"ai-agent@localhost\". Use force: true to override.", authorEmail)
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "git_rollback",
				Path:       args.Workdir,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}
		if !ageOk {
			err = fmt.Errorf("safety check failed: most recent agent commit is older than 24 hours (age: %v). Use force: true to override.", age)
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "git_rollback",
				Path:       args.Workdir,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}
	}

	resetCmd := fmt.Sprintf("cd %s && git reset --hard HEAD~%d && git clean -fd", shellQuote(args.Workdir), commitsBack)
	resetRes, err := remoteExec(ctx, client, resetCmd)
	if err != nil || resetRes.ExitCode != 0 {
		var exitCode int
		if resetRes != nil {
			exitCode = resetRes.ExitCode
		}
		var stderr string
		if resetRes != nil {
			stderr = resetRes.Stderr
		}
		err = fmt.Errorf("failed to execute rollback: %v (exit code %d, stderr: %s)", err, exitCode, stderr)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "git_rollback",
			Path:       args.Workdir,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	auditLog(AuditEntry{
		Profile:    profile.Alias,
		Host:       profile.Host,
		Tool:       "git_rollback",
		Path:       args.Workdir,
		DurationMs: time.Since(start).Milliseconds(),
	})

	return textContent(fmt.Sprintf("✓ Successfully rolled back %d commit(s) in %s.\n%s", commitsBack, args.Workdir, formatExecResult(resetRes))), nil, nil
}

func handleSshPortForward(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args SshPortForwardArgs,
) (*mcp.CallToolResult, any, error) {
	switch strings.ToLower(args.Action) {
	case "list":
		tunnelsMu.Lock()
		defer tunnelsMu.Unlock()
		if len(tunnels) == 0 {
			return textContent("No active SSH tunnels."), nil, nil
		}
		var sb strings.Builder
		sb.WriteString("Active SSH Tunnels:\n")
		for port, t := range tunnels {
			fmt.Fprintf(&sb, "- localhost:%d -> %s:%d\n", port, t.RemoteHost, t.RemotePort)
		}
		return textContent(sb.String()), nil, nil

	case "stop":
		if args.LocalPort <= 0 {
			return errContent("'local_port' is required to stop forwarding"), nil, nil
		}
		tunnelsMu.Lock()
		t, ok := tunnels[args.LocalPort]
		if ok {
			delete(tunnels, args.LocalPort)
		}
		tunnelsMu.Unlock()

		if !ok {
			return errContent("No active tunnel found on local port %d", args.LocalPort), nil, nil
		}

		t.Cancel()
		t.Listener.Close()
		return textContent(fmt.Sprintf("✓ Stopped forwarding local port %d", args.LocalPort)), nil, nil

	case "start":
		if args.LocalPort <= 0 {
			return errContent("'local_port' is required to start forwarding"), nil, nil
		}
		if args.RemotePort <= 0 {
			return errContent("'remote_port' is required to start forwarding"), nil, nil
		}
		remoteHost := args.RemoteHost
		if remoteHost == "" {
			remoteHost = "localhost"
		}

		// Port conflict is checked atomically during registration below.

		profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
		if err != nil {
			return errContent("profile error: %v", err), nil, nil
		}

		if err := checkRateLimitForProfile(profile); err != nil {
			return errContent("%v", err), nil, nil
		}

		if _, err := getOrConnect(profile); err != nil {
			return errContent("connection failed: %v", err), nil, nil
		}

		localAddr := fmt.Sprintf("127.0.0.1:%d", args.LocalPort)
		listener, err := net.Listen("tcp", localAddr)
		if err != nil {
			return errContent("failed to bind local port %d: %v", args.LocalPort, err), nil, nil
		}

		tunnelCtx, cancel := context.WithCancel(context.Background())
		t := &ForwardingTunnel{
			LocalPort:  args.LocalPort,
			RemoteHost: remoteHost,
			RemotePort: args.RemotePort,
			Listener:   listener,
			Cancel:     cancel,
		}

		tunnelsMu.Lock()
		if _, alreadyActive := tunnels[args.LocalPort]; alreadyActive {
			tunnelsMu.Unlock()
			listener.Close()
			cancel()
			return errContent("Local port %d was claimed by another tunnel during setup", args.LocalPort), nil, nil
		}
		tunnels[args.LocalPort] = t
		tunnelsMu.Unlock()

		go func() {
			defer func() {
				listener.Close()
				tunnelsMu.Lock()
				delete(tunnels, args.LocalPort)
				tunnelsMu.Unlock()
			}()

			for {
				localConn, err := listener.Accept()
				if err != nil {
					select {
					case <-tunnelCtx.Done():
						return
					default:
						log.Printf("[warn] accept failed on port %d: %v", args.LocalPort, err)
						return
					}
				}

				go func(lConn net.Conn) {
					defer lConn.Close()

					sshClient, err := getOrConnect(profile)
					if err != nil {
						log.Printf("[warn] tunnel dial failed: client reconnect failed: %v", err)
						return
					}

					remoteConn, err := sshClient.Dial("tcp", fmt.Sprintf("%s:%d", remoteHost, args.RemotePort))
					if err != nil {
						log.Printf("[warn] tunnel dial failed to %s:%d: %v", remoteHost, args.RemotePort, err)
						return
					}
					defer remoteConn.Close()

					chDone := make(chan struct{}, 2)
					go func() {
						_, _ = io.Copy(remoteConn, lConn)
						chDone <- struct{}{}
					}()
					go func() {
						_, _ = io.Copy(lConn, remoteConn)
						chDone <- struct{}{}
					}()

					select {
					case <-chDone:
					case <-tunnelCtx.Done():
					}
				}(localConn)
			}
		}()

		return textContent(fmt.Sprintf("✓ Started SSH tunnel: localhost:%d -> %s:%d (via %s)", args.LocalPort, remoteHost, args.RemotePort, profile.Host)), nil, nil

	default:
		return errContent("invalid action %q (must be start, stop, or list)", args.Action), nil, nil
	}
}

func handleSecureFileTransfer(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args SecureFileTransferArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	direction := strings.ToLower(args.Direction)

	// 1. Enforce Allowed Paths
	if !isPathAllowed(profile.AllowedPaths, args.RemotePath) {
		err = fmt.Errorf("path %s is not within allowed paths for this profile", args.RemotePath)
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_transfer",
			Operation:  direction,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	// 2. Enforce ReadOnly check
	if direction == "upload" && profile.ReadOnly {
		err = fmt.Errorf("profile is configured as read-only; write operations are not permitted")
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_transfer",
			Operation:  direction,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "secure_file_transfer",
			Operation:  direction,
			Path:       args.RemotePath,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("connection failed: %v", err), nil, nil
	}

	n, errTransfer := transferFileStream(client, args.LocalPath, args.RemotePath, direction)
	var result *mcp.CallToolResult
	if errTransfer != nil {
		err = errTransfer
		result = errContent("transfer error: %v", err)
	} else {
		if direction == "upload" {
			result = textContent(fmt.Sprintf("✓ Uploaded %d bytes from %s to %s", n, args.LocalPath, args.RemotePath))
		} else {
			result = textContent(fmt.Sprintf("✓ Downloaded %d bytes from %s to %s", n, args.RemotePath, args.LocalPath))
		}
	}

	var errMsg string
	if err != nil {
		errMsg = err.Error()
	} else if result != nil && result.IsError {
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				errMsg = tc.Text
			}
		}
	}

	auditLog(AuditEntry{
		Profile:          profile.Alias,
		Host:             profile.Host,
		Tool:             "secure_file_transfer",
		Operation:        direction,
		Path:             args.RemotePath,
		BytesTransferred: n,
		DurationMs:       time.Since(start).Milliseconds(),
		Error:            errMsg,
	})

	return result, nil, nil
}

func handleGetSystemVitals(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args GetSystemVitalsArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "get_system_vitals",
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("connection failed: %v", err), nil, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	vitals, _ := fetchVitals(execCtx, client)

	rawJSON, err := json.MarshalIndent(vitals, "", "  ")
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "get_system_vitals",
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("failed to serialize vitals to JSON: %v", err), nil, nil
	}

	auditLog(AuditEntry{
		Profile:    profile.Alias,
		Host:       profile.Host,
		Tool:       "get_system_vitals",
		DurationMs: time.Since(start).Milliseconds(),
	})

	return textContent(string(rawJSON)), nil, nil
}

func handleManageRemoteProcess(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args ManageRemoteProcessArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	action := strings.ToLower(args.Action)

	// 1. Enforce ReadOnly check
	if action == "start" && profile.ReadOnly {
		err = fmt.Errorf("profile is configured as read-only; write operations are not permitted")
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "manage_remote_process",
			Action:     action,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "manage_remote_process",
			Action:     action,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return errContent("connection failed: %v", err), nil, nil
	}

	switch action {
	case "start":
		if args.Command == "" {
			err = fmt.Errorf("'command' is required to start a background process")
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		cleanCmd, errCmd := validateCommandPolicy(profile, args.Command)
		if errCmd != nil {
			err = errCmd
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("command rejection: %v", err), nil, nil
		}

		processID := args.ProcessID
		if processID == "" {
			processID = generateProcessID()
		} else if !isValidProcessID(processID) {
			err = fmt.Errorf("invalid process_id structure. Must be alphanumeric, underscores, or dashes only.")
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		sfClient, errSftp := sftp.NewClient(client)
		if errSftp != nil {
			err = errSftp
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("SFTP session error: %v", err), nil, nil
		}
		defer sfClient.Close()

		wd, errWd := sfClient.Getwd()
		if errWd != nil {
			err = errWd
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("SFTP getwd error: %v", err), nil, nil
		}

		targetDir := path.Join(wd, ".ai_ssh_processes", processID)
		if errMkdir := sfClient.MkdirAll(targetDir); errMkdir != nil {
			err = errMkdir
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to create remote directory: %v", err), nil, nil
		}

		runScriptPath := path.Join(targetDir, "run.sh")
		f, errCreate := sfClient.Create(runScriptPath)
		if errCreate != nil {
			err = errCreate
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to create run.sh script: %v", err), nil, nil
		}

		var scriptContent strings.Builder
		scriptContent.WriteString("#!/bin/bash\n")
		if args.Workdir != "" {
			scriptContent.WriteString(fmt.Sprintf("cd %q || exit 1\n", args.Workdir))
		}
		scriptContent.WriteString(fmt.Sprintf("%s\n", cleanCmd))
		scriptContent.WriteString(fmt.Sprintf("echo $? > %q\n", path.Join(targetDir, "exit_code")))

		_, errWrite := f.Write([]byte(scriptContent.String()))
		f.Close()
		if errWrite != nil {
			err = errWrite
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to write run.sh script: %v", err), nil, nil
		}

		if errChmod := sfClient.Chmod(runScriptPath, 0755); errChmod != nil {
			err = errChmod
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to chmod run.sh script: %v", err), nil, nil
		}

		startCmd := fmt.Sprintf(
			"nohup bash %q > %q 2>&1 & echo $! > %q",
			runScriptPath,
			path.Join(targetDir, "log"),
			path.Join(targetDir, "pid"),
		)

		res, errExec := remoteExec(ctx, client, startCmd)
		if errExec != nil || res.ExitCode != 0 {
			var exitCode int
			if res != nil {
				exitCode = res.ExitCode
			}
			var stderr string
			if res != nil {
				stderr = res.Stderr
			}
			err = fmt.Errorf("failed to start background task: %v (exit code %d, stderr: %s)", errExec, exitCode, stderr)
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		pidRes, errPid := remoteExec(ctx, client, fmt.Sprintf("cat %q 2>/dev/null", path.Join(targetDir, "pid")))
		pidStr := strings.TrimSpace(pidRes.Stdout)
		if errPid != nil || pidRes.ExitCode != 0 || pidStr == "" {
			err = fmt.Errorf("background task failed to write PID file: %v", errPid)
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		pid, _ := strconv.Atoi(pidStr)

		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "manage_remote_process",
			Action:     action,
			ProcessID:  processID,
			DurationMs: time.Since(start).Milliseconds(),
		})

		respMap := map[string]any{
			"process_id": processID,
			"pid":        pid,
			"status":     "running",
			"message":    fmt.Sprintf("Process started under PID %d. Logs are stored in %s/log", pid, targetDir),
		}
		rawJSON, _ := json.MarshalIndent(respMap, "", "  ")
		return textContent(string(rawJSON)), nil, nil

	case "status":
		processID := args.ProcessID
		if !isValidProcessID(processID) {
			err = fmt.Errorf("valid 'process_id' is required for status check")
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		statusScript := fmt.Sprintf(`
dir=~/.ai_ssh_processes/%s
if [ ! -d "$dir" ]; then
    echo "not_found"
    exit 0
fi
pid=$(cat "$dir/pid" 2>/dev/null)
if [ -z "$pid" ]; then
    echo "no_pid"
    exit 0
fi
if kill -0 "$pid" 2>/dev/null; then
    echo "running $pid"
else
    if [ -f "$dir/exit_code" ]; then
        echo "finished $pid $(cat "$dir/exit_code" 2>/dev/null)"
    else
        echo "terminated $pid"
    fi
fi`, processID)

		res, errExec := remoteExec(ctx, client, statusScript)
		if errExec != nil {
			err = errExec
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to fetch background process status: %v", err), nil, nil
		}

		fields := strings.Fields(res.Stdout)
		if len(fields) == 0 {
			err = fmt.Errorf("status script returned empty output")
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		statusMap := map[string]any{"process_id": processID}
		switch fields[0] {
		case "not_found":
			statusMap["status"] = "not_found"
			statusMap["message"] = "Process state directory not found."
		case "no_pid":
			statusMap["status"] = "no_pid"
			statusMap["message"] = "PID file empty or missing."
		case "running":
			statusMap["status"] = "running"
			if len(fields) > 1 {
				pid, _ := strconv.Atoi(fields[1])
				statusMap["pid"] = pid
			}
		case "finished":
			statusMap["status"] = "finished"
			if len(fields) > 1 {
				pid, _ := strconv.Atoi(fields[1])
				statusMap["pid"] = pid
			}
			if len(fields) > 2 {
				ec, _ := strconv.Atoi(fields[2])
				statusMap["exit_code"] = ec
			}
		case "terminated":
			statusMap["status"] = "terminated"
			if len(fields) > 1 {
				pid, _ := strconv.Atoi(fields[1])
				statusMap["pid"] = pid
			}
			statusMap["message"] = "Process died or was killed without writing exit code."
		}

		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "manage_remote_process",
			Action:     action,
			ProcessID:  processID,
			DurationMs: time.Since(start).Milliseconds(),
		})

		rawJSON, _ := json.MarshalIndent(statusMap, "", "  ")
		return textContent(string(rawJSON)), nil, nil

	case "logs":
		processID := args.ProcessID
		if !isValidProcessID(processID) {
			err = fmt.Errorf("valid 'process_id' is required for logs check")
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}
		lines := args.Lines
		if lines <= 0 {
			lines = 100
		}

		logCmd := fmt.Sprintf("tail -n %d ~/.ai_ssh_processes/%s/log 2>/dev/null || echo \"[No logs found]\"", lines, processID)
		res, errExec := remoteExec(ctx, client, logCmd)
		if errExec != nil {
			err = errExec
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to read background logs: %v", err), nil, nil
		}

		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "manage_remote_process",
			Action:     action,
			ProcessID:  processID,
			DurationMs: time.Since(start).Milliseconds(),
		})

		return textContent(res.Stdout), nil, nil

	case "stop":
		processID := args.ProcessID
		if !isValidProcessID(processID) {
			err = fmt.Errorf("valid 'process_id' is required to stop the process")
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("%v", err), nil, nil
		}

		stopScript := fmt.Sprintf(`
dir=~/.ai_ssh_processes/%s
pid=$(cat "$dir/pid" 2>/dev/null)
if [ ! -z "$pid" ]; then
    if kill -0 "$pid" 2>/dev/null; then
        kill -15 "$pid" 2>/dev/null || true
        sleep 1
        kill -9 "$pid" 2>/dev/null || true
        echo "killed $pid"
    else
        echo "not_running $pid"
    fi
else
    echo "no_pid"
fi`, processID)

		res, errExec := remoteExec(ctx, client, stopScript)
		if errExec != nil {
			err = errExec
			auditLog(AuditEntry{
				Profile:    profile.Alias,
				Host:       profile.Host,
				Tool:       "manage_remote_process",
				Action:     action,
				ProcessID:  processID,
				DurationMs: time.Since(start).Milliseconds(),
				Error:      err.Error(),
			})
			return errContent("failed to stop background process: %v", err), nil, nil
		}

		_, _ = remoteExec(ctx, client, fmt.Sprintf("echo -1 > ~/.ai_ssh_processes/%s/exit_code", processID))

		auditLog(AuditEntry{
			Profile:    profile.Alias,
			Host:       profile.Host,
			Tool:       "manage_remote_process",
			Action:     action,
			ProcessID:  processID,
			DurationMs: time.Since(start).Milliseconds(),
		})

		return textContent(strings.TrimSpace(res.Stdout)), nil, nil
	}

	err = fmt.Errorf("invalid action %q (must be start, status, logs, or stop)", args.Action)
	return errContent("%v", err), nil, nil
}

// handleListProfiles lists all connection profile configurations, redacting secrets.
func handleListProfiles(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args ListProfilesArgs,
) (*mcp.CallToolResult, any, error) {
	profileRegistryMu.RLock()
	defer profileRegistryMu.RUnlock()

	type PublicProfile struct {
		Alias           string   `json:"alias"`
		Host            string   `json:"host"`
		Port            int      `json:"port"`
		User            string   `json:"user"`
		UseAgent        bool     `json:"use_agent"`
		GitEnabled      bool     `json:"git_enabled"`
		AllowedCommands []string `json:"allowed_commands,omitempty"`
		BlockedCommands []string `json:"blocked_commands,omitempty"`
		HostKey         string   `json:"host_key,omitempty"`
		ReadOnly        bool     `json:"readonly"`
		RateLimitRPM    *int     `json:"rate_limit_rpm,omitempty"`
		AllowedPaths    []string `json:"allowed_paths,omitempty"`
	}

	var list []PublicProfile
	for _, p := range profileRegistry {
		list = append(list, PublicProfile{
			Alias:           p.Alias,
			Host:            p.Host,
			Port:            p.Port,
			User:            p.User,
			UseAgent:        p.UseAgent,
			GitEnabled:      p.GitEnabled,
			AllowedCommands: p.AllowedCommands,
			BlockedCommands: p.BlockedCommands,
			HostKey:         p.HostKey,
			ReadOnly:        p.ReadOnly,
			RateLimitRPM:    p.RateLimitRPM,
			AllowedPaths:    p.AllowedPaths,
		})
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return errContent("marshal failed: %v", err), nil, nil
	}

	return textContent(string(data)), nil, nil
}

// handleSaveSshProfile dynamically saves/updates a connection profile.
func handleSaveSshProfile(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args SaveSshProfileArgs,
) (*mcp.CallToolResult, any, error) {
	if args.Alias == "" {
		return errContent("alias is required"), nil, nil
	}
	if args.Host == "" || args.User == "" {
		return errContent("host and user are required"), nil, nil
	}

	// Profile writes are denied by default: the profile is where every other
	// guardrail lives, so an agent that can rewrite profiles can disarm them all.
	if !profileWritesEnabled() {
		auditLog(AuditEntry{
			Profile: args.Alias,
			Host:    args.Host,
			Tool:    "save_ssh_profile",
			Status:  "denied",
			Error:   "profile writes disabled",
		})
		return errContent(
			"profile writes over MCP are disabled. Set %s=1 in the server environment to enable them, "+
				"or add the profile with the CLI: ai-ssh-tools profiles",
			profileWritesEnvVar,
		), nil, nil
	}

	port := args.Port
	if port == 0 {
		port = 22
	}

	p := HostProfile{
		Alias:           args.Alias,
		Host:            args.Host,
		Port:            port,
		User:            args.User,
		KeyPath:         args.KeyPath,
		Password:        args.Password,
		UseAgent:        args.UseAgent,
		GitEnabled:      args.GitEnabled,
		AllowedCommands: args.AllowedCommands,
		BlockedCommands: args.BlockedCommands,
		HostKey:         args.HostKey,
		ReadOnly:        args.ReadOnly,
		RateLimitRPM:    args.RateLimitRPM,
		AllowedPaths:    args.AllowedPaths,
	}

	// Even with writes enabled, an existing profile may only be made stricter.
	profileRegistryMu.RLock()
	existing, exists := profileRegistry[args.Alias]
	profileRegistryMu.RUnlock()
	if exists {
		if err := checkProfileWeakening(existing, &p); err != nil {
			auditLog(AuditEntry{
				Profile: args.Alias,
				Host:    args.Host,
				Tool:    "save_ssh_profile",
				Status:  "denied",
				Error:   err.Error(),
			})
			return errContent("%v", err), nil, nil
		}
	}

	if err := saveProfile(p); err != nil {
		return errContent("failed to save profile: %v", err), nil, nil
	}

	return textContent(fmt.Sprintf("Profile %q successfully saved and reloaded.", args.Alias)), nil, nil
}

// handleDockerContainers inspects Docker containers on the remote host.
func handleDockerContainers(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args DockerContainersArgs,
) (*mcp.CallToolResult, any, error) {
	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		return errContent("connection failed: %v", err), nil, nil
	}

	cmd := `docker ps --format "{{json .}}"`
	if args.All {
		cmd = `docker ps -a --format "{{json .}}"`
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res, err := remoteExec(execCtx, client, cmd)
	if err != nil {
		return errContent("docker execution error: %v", err), nil, nil
	}

	if res.ExitCode != 0 {
		return errContent("docker failed (exit %d): %s", res.ExitCode, res.Stderr), nil, nil
	}

	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	var containers []map[string]any
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(l), &item); err == nil {
			containers = append(containers, item)
		}
	}

	out, err := json.MarshalIndent(containers, "", "  ")
	if err != nil {
		return textContent(res.Stdout), nil, nil
	}

	return textContent(string(out)), nil, nil
}

// handleManageService manages and inspects remote systemd services.
func handleManageService(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args ManageServiceArgs,
) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	if args.Name == "" {
		return errContent("service name is required"), nil, nil
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	if action == "" {
		action = "status"
	}

	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if profile.ReadOnly && action != "status" && action != "logs" {
		return errContent("profile is read-only; service action %q is not permitted", action), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		return errContent("connection failed: %v", err), nil, nil
	}

	lines := args.Lines
	if lines <= 0 {
		lines = 50
	}

	var cmd string
	switch action {
	case "status":
		cmd = fmt.Sprintf("systemctl status %s --no-pager", shellQuote(args.Name))
	case "start", "stop", "restart", "enable", "disable", "reload":
		cmd = fmt.Sprintf("systemctl %s %s", action, shellQuote(args.Name))
	case "logs":
		cmd = fmt.Sprintf("journalctl -u %s -n %d --no-pager", shellQuote(args.Name), lines)
	default:
		return errContent("invalid action %q (supported: status, start, stop, restart, enable, disable, reload, logs)", action), nil, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res, err := remoteExecOpts(execCtx, client, cmd, false, args.Sudo, args.SudoPassword)
	var exitCode *int
	if res != nil {
		exitCode = &res.ExitCode
	}
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	auditLog(AuditEntry{
		Profile: profile.Alias, Host: profile.Host, Tool: "manage_service", Action: action,
		Command: cmd, ExitCode: exitCode, DurationMs: time.Since(start).Milliseconds(), Error: errMsg,
	})
	if err != nil {
		return errContent("service command failed: %v", err), nil, nil
	}

	return textContent(formatExecResult(res)), nil, nil
}

// handleTailRemoteFile reads the last lines of a remote log file.
func handleTailRemoteFile(
	ctx context.Context,
	req *mcp.CallToolRequest,
	args TailRemoteFileArgs,
) (*mcp.CallToolResult, any, error) {
	if args.Path == "" {
		return errContent("path is required"), nil, nil
	}

	profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
	if err != nil {
		return errContent("profile error: %v", err), nil, nil
	}

	if !isPathAllowed(profile.AllowedPaths, args.Path) {
		return errContent("path %q is not in the allowed paths list", args.Path), nil, nil
	}

	if err := checkRateLimitForProfile(profile); err != nil {
		return errContent("%v", err), nil, nil
	}

	client, err := getOrConnect(profile)
	if err != nil {
		return errContent("connection failed: %v", err), nil, nil
	}

	lines := args.Lines
	if lines <= 0 {
		lines = 50
	} else if lines > 1000 {
		lines = 1000
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := fmt.Sprintf("tail -n %d %s", lines, shellQuote(args.Path))
	res, err := remoteExec(execCtx, client, cmd)
	if err != nil {
		return errContent("tail failed: %v", err), nil, nil
	}

	return textContent(formatExecResult(res)), nil, nil
}
