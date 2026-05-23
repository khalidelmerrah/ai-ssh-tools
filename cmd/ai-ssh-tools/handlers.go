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
	Profile        string `json:"profile,omitempty" jsonschema:"description=Named profile alias from ssh_hosts.json (mutually exclusive with host/user)"`
	Host           string `json:"host,omitempty" jsonschema:"description=Remote hostname or IP (used when profile is not specified)"`
	User           string `json:"user,omitempty" jsonschema:"description=SSH username (used when profile is not specified)"`
	Port           int    `json:"port,omitempty" jsonschema:"description=SSH port — defaults to 22 (used when profile is not specified)"`
	Command        string `json:"command" jsonschema:"required,description=Shell command to execute on the remote host. Must be a single atomic command with no shell chaining operators."`
	Workdir        string `json:"workdir,omitempty"     jsonschema:"description=Absolute path on the remote host to use as the working directory. When set, the command is wrapped in pre/post git snapshots for rollback safety."`
	GitWrapped     bool   `json:"git_wrapped,omitempty" jsonschema:"description=When true and workdir is set, wraps execution in pre/post git snapshots (requires git on the remote)."`
	TimeoutSeconds *int   `json:"timeout_seconds,omitempty" jsonschema:"description=Timeout in seconds for command execution (default 30, max 300)"`
}

// SecureFileDeltaArgs are the typed inputs for the secure_file_delta tool.
type SecureFileDeltaArgs struct {
	Profile    string `json:"profile,omitempty"      jsonschema:"description=Named profile alias from ssh_hosts.json"`
	Host       string `json:"host,omitempty"         jsonschema:"description=Remote hostname or IP"`
	User       string `json:"user,omitempty"         jsonschema:"description=SSH username"`
	Port       int    `json:"port,omitempty"         jsonschema:"description=SSH port (default 22)"`
	Operation  string `json:"operation"              jsonschema:"required,enum=read;write;list,description=File operation: read (download content), write (upload content), list (list directory)"`
	RemotePath string `json:"remote_path"            jsonschema:"required,description=Absolute path on the remote host"`
	Content    string `json:"content,omitempty"      jsonschema:"description=UTF-8 file content to write (only for write operation)"`
	MaxBytes   int64  `json:"max_bytes,omitempty"    jsonschema:"description=Maximum bytes to read (default 131072 / 128 KB to keep LLM context manageable)"`
}

// GitRollbackArgs are the typed inputs for the git_rollback tool.
type GitRollbackArgs struct {
	Profile     string `json:"profile,omitempty"      jsonschema:"description=Named profile alias from ssh_hosts.json"`
	Host        string `json:"host,omitempty"         jsonschema:"description=Remote hostname or IP"`
	User        string `json:"user,omitempty"         jsonschema:"description=SSH username"`
	Port        int    `json:"port,omitempty"         jsonschema:"description=SSH port (default 22)"`
	Workdir     string `json:"workdir"                 jsonschema:"required,description=Absolute path on the remote host to the git repository root"`
	CommitsBack int    `json:"commits_back,omitempty" jsonschema:"description=Number of commits to roll back (default 2, which undoes the pre and post changes of the last agent execution)"`
	Force       bool   `json:"force,omitempty"        jsonschema:"description=Force rollback even if safety checks fail"`
}

// SshPortForwardArgs are the typed inputs for the ssh_port_forward tool.
type SshPortForwardArgs struct {
	Profile    string `json:"profile,omitempty"   jsonschema:"description=Named profile alias from ssh_hosts.json"`
	Host       string `json:"host,omitempty"      jsonschema:"description=Remote hostname or IP"`
	User       string `json:"user,omitempty"      jsonschema:"description=SSH username"`
	Port       int    `json:"port,omitempty"      jsonschema:"description=SSH port (default 22)"`
	Action     string `json:"action"              jsonschema:"required,enum=start;stop;list,description=Action: start, stop, or list tunnels"`
	LocalPort  int    `json:"local_port,omitempty" jsonschema:"description=Local port to bind on client machine"`
	RemoteHost string `json:"remote_host,omitempty" jsonschema:"description=Target host reachable from the remote SSH server (default: localhost)"`
	RemotePort int    `json:"remote_port,omitempty" jsonschema:"description=Target port on the remote host"`
}

// SecureFileTransferArgs are the typed inputs for the secure_file_transfer tool.
type SecureFileTransferArgs struct {
	Profile    string `json:"profile,omitempty"   jsonschema:"description=Named profile alias from ssh_hosts.json"`
	Host       string `json:"host,omitempty"      jsonschema:"description=Remote hostname or IP"`
	User       string `json:"user,omitempty"      jsonschema:"description=SSH username"`
	Port       int    `json:"port,omitempty"      jsonschema:"description=SSH port (default 22)"`
	Direction  string `json:"direction"           jsonschema:"required,enum=upload;download,description=Transfer direction: upload or download"`
	LocalPath  string `json:"local_path"          jsonschema:"required,description=Absolute local file path on the client machine"`
	RemotePath string `json:"remote_path"         jsonschema:"required,description=Absolute remote file path on the server"`
}

// GetSystemVitalsArgs are the typed inputs for the get_system_vitals tool.
type GetSystemVitalsArgs struct {
	Profile string `json:"profile,omitempty" jsonschema:"description=Named profile alias from ssh_hosts.json"`
	Host    string `json:"host,omitempty"    jsonschema:"description=Remote hostname or IP"`
	User    string `json:"user,omitempty"    jsonschema:"description=SSH username"`
	Port    int    `json:"port,omitempty"    jsonschema:"description=SSH port (default 22)"`
}

// ManageRemoteProcessArgs are the typed inputs for the manage_remote_process tool.
type ManageRemoteProcessArgs struct {
	Profile   string `json:"profile,omitempty"   jsonschema:"description=Named profile alias from ssh_hosts.json"`
	Host      string `json:"host,omitempty"      jsonschema:"description=Remote hostname or IP"`
	User      string `json:"user,omitempty"      jsonschema:"description=SSH username"`
	Port      int    `json:"port,omitempty"      jsonschema:"description=SSH port (default 22)"`
	Action    string `json:"action"              jsonschema:"required,enum=start;status;stop;logs,description=Process action: start (run in background), status (check execution status), stop (kill process), logs (read stdout/stderr logs)"`
	Command   string `json:"command,omitempty"   jsonschema:"description=The command to execute (required only for start)"`
	ProcessID string `json:"process_id,omitempty" jsonschema:"description=Unique identifier of the process (required for status, stop, logs)"`
	Lines     int    `json:"lines,omitempty"      jsonschema:"description=Number of tail lines of logs to retrieve (default 100)"`
	Workdir   string `json:"workdir,omitempty"    jsonschema:"description=Working directory on the remote server to start the process in"`
}

// ─────────────────────────────────────────────────────────────────────────────
// PROFILE RESOLUTION
// ─────────────────────────────────────────────────────────────────────────────

func resolveProfile(profileAlias, host, user string, port int) (*HostProfile, error) {
	if profileAlias != "" {
		p, ok := profileRegistry[profileAlias]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q — check ssh_hosts.json", profileAlias)
		}
		return p, nil
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
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new SSH session: %w", err)
	}
	defer sess.Close()

	_ = sess.Setenv("DEBIAN_FRONTEND", "noninteractive")
	_ = sess.Setenv("TERM", "dumb")

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
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
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}, nil
	}
}

func gitWrappedExec(ctx context.Context, client *ssh.Client, workdir, cmd string) (*execResult, error) {
	if _, err := remoteExec(ctx, client, fmt.Sprintf("cd %s && git init -q", workdir)); err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}

	_, _ = remoteExec(ctx, client, `git config --global user.email "ai-agent@localhost" 2>/dev/null || true`)
	_, _ = remoteExec(ctx, client, `git config --global user.name "AI Agent" 2>/dev/null || true`)

	preSnap := fmt.Sprintf(
		`cd %s && git add -A 2>/dev/null; git commit --allow-empty -m "Pre-agent snapshot" -q 2>/dev/null || true`,
		workdir,
	)
	if _, err := remoteExec(ctx, client, preSnap); err != nil {
		log.Printf("[warn] pre-snapshot failed: %v", err)
	}

	result, execErr := remoteExec(ctx, client, fmt.Sprintf("cd %s && %s", workdir, cmd))

	sanitizedMsg := strings.ReplaceAll(cmd, `"`, `'`)
	if len(sanitizedMsg) > 72 {
		sanitizedMsg = sanitizedMsg[:72] + "..."
	}
	postSnap := fmt.Sprintf(
		`cd %s && git add -A 2>/dev/null; git commit --allow-empty -m "AI Auto-save: %s" -q 2>/dev/null || true`,
		workdir, sanitizedMsg,
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

	if profile.ReadOnly {
		return errContent("profile is configured as read-only; write operations are not permitted"), nil, nil
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
		result, err = gitWrappedExec(execCtx, client, args.Workdir, cleanCmd)
	} else {
		execCmd := cleanCmd
		if args.Workdir != "" {
			execCmd = fmt.Sprintf("cd %s && %s", args.Workdir, cleanCmd)
		}
		result, err = remoteExec(execCtx, client, execCmd)
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

	res, err := remoteExec(ctx, client, fmt.Sprintf("cd %s && git rev-parse --is-inside-work-tree", args.Workdir))
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

	logRes, err := remoteExec(ctx, client, fmt.Sprintf("cd %s && git log -n 1 --format=%%ae|%%ct", args.Workdir))
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

	resetCmd := fmt.Sprintf("cd %s && git reset --hard HEAD~%d && git clean -fd", args.Workdir, commitsBack)
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

		tunnelsMu.Lock()
		_, active := tunnels[args.LocalPort]
		tunnelsMu.Unlock()
		if active {
			return errContent("Local port %d is already in use by another SSH tunnel", args.LocalPort), nil, nil
		}

		profile, err := resolveProfile(args.Profile, args.Host, args.User, args.Port)
		if err != nil {
			return errContent("profile error: %v", err), nil, nil
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

	sfClient, err := sftp.NewClient(client)
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
		return errContent("SFTP session error: %v", err), nil, nil
	}
	defer sfClient.Close()

	var result *mcp.CallToolResult
	if direction == "upload" {
		localFile, errOpen := os.Open(args.LocalPath)
		if errOpen != nil {
			err = errOpen
			result = errContent("failed to open local file %q: %v", args.LocalPath, err)
		} else {
			defer localFile.Close()

			dir := filepath.ToSlash(filepath.Dir(args.RemotePath))
			if errMkdir := sfClient.MkdirAll(dir); errMkdir != nil {
				err = errMkdir
				result = errContent("mkdirall %s: %v", dir, err)
			} else {
				remoteFile, errCreate := sfClient.Create(args.RemotePath)
				if errCreate != nil {
					err = errCreate
					result = errContent("failed to create remote file %q: %v", args.RemotePath, err)
				} else {
					defer remoteFile.Close()

					n, errCopy := io.Copy(remoteFile, localFile)
					if errCopy != nil {
						err = errCopy
						result = errContent("failed to upload bytes: %v", err)
					} else {
						result = textContent(fmt.Sprintf("✓ Uploaded %d bytes from %s to %s", n, args.LocalPath, args.RemotePath))
					}
				}
			}
		}
	} else if direction == "download" {
		remoteFile, errOpen := sfClient.Open(args.RemotePath)
		if errOpen != nil {
			err = errOpen
			result = errContent("failed to open remote file %q: %v", args.RemotePath, err)
		} else {
			defer remoteFile.Close()

			dir := filepath.Dir(args.LocalPath)
			if errMkdir := os.MkdirAll(dir, 0755); errMkdir != nil {
				err = errMkdir
				result = errContent("failed to create local directories %q: %v", dir, err)
			} else {
				localFile, errCreate := os.Create(args.LocalPath)
				if errCreate != nil {
					err = errCreate
					result = errContent("failed to create local file %q: %v", args.LocalPath, err)
				} else {
					defer localFile.Close()

					n, errCopy := io.Copy(localFile, remoteFile)
					if errCopy != nil {
						err = errCopy
						result = errContent("failed to download bytes: %v", err)
					} else {
						result = textContent(fmt.Sprintf("✓ Downloaded %d bytes from %s to %s", n, args.RemotePath, args.LocalPath))
					}
				}
			}
		}
	} else {
		err = fmt.Errorf("invalid direction %q (must be upload or download)", args.Direction)
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
		Tool:       "secure_file_transfer",
		Operation:  direction,
		Path:       args.RemotePath,
		DurationMs: time.Since(start).Milliseconds(),
		Error:      errMsg,
	})

	return result, nil, err
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

	osRes, _ := remoteExec(ctx, client, "cat /etc/os-release")
	osName := "Linux (Unknown)"
	if osRes != nil && osRes.ExitCode == 0 {
		osName = parseOSName(osRes.Stdout)
	}

	uptimeRes, _ := remoteExec(ctx, client, "cat /proc/uptime")
	var uptime int64
	if uptimeRes != nil && uptimeRes.ExitCode == 0 {
		uptime = parseUptime(uptimeRes.Stdout)
	}

	loadRes, _ := remoteExec(ctx, client, "cat /proc/loadavg")
	var loads []float64
	if loadRes != nil && loadRes.ExitCode == 0 {
		loads = parseLoadAverages(loadRes.Stdout)
	}

	memRes, _ := remoteExec(ctx, client, "free -b")
	var mem MemoryVitals
	if memRes != nil && memRes.ExitCode == 0 {
		mem = parseMemoryBytes(memRes.Stdout)
	}

	diskRes, _ := remoteExec(ctx, client, "df -B1")
	var disks []DiskVitals
	if diskRes != nil && diskRes.ExitCode == 0 {
		disks = parseDisks(diskRes.Stdout)
	}

	vitals := SystemVitals{
		OSName:        osName,
		UptimeSeconds: uptime,
		LoadAverages:  loads,
		MemoryBytes:   mem,
		Disks:         disks,
	}

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
