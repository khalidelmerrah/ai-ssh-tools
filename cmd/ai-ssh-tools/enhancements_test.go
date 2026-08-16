package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/ssh"
)

func setupTempHome(t *testing.T) string {
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")

	os.Setenv("HOME", tempDir)
	os.Setenv("USERPROFILE", tempDir)

	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldUserProfile)
	})
	return tempDir
}

func startMockSSHServer(t *testing.T) (net.Listener, *ssh.ServerConfig) {
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("failed to add private key: %v", err)
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			_, chans, reqs, err := ssh.NewServerConn(nConn, config)
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(reqs)
			go func() {
				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					channel, requests, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range requests {
							switch req.Type {
							case "exec":
								// Simulate long command by hanging
								select {}
							default:
								req.Reply(true, nil)
							}
						}
						channel.Close()
					}()
				}
			}()
		}
	}()

	return listener, config
}

func TestTOFU(t *testing.T) {
	setupTempHome(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	pubKey := signer.PublicKey()

	callback := getTOFUHostKeyCallback("test-user@localhost:22")

	// 1. First connect: saves fingerprint and succeeds
	err = callback("localhost:22", nil, pubKey)
	if err != nil {
		t.Fatalf("expected first connection to succeed, got: %v", err)
	}

	// 2. Subsequent connect with same key: succeeds
	err = callback("localhost:22", nil, pubKey)
	if err != nil {
		t.Fatalf("expected subsequent connection with same key to succeed, got: %v", err)
	}

	// 3. Subsequent connect with different key: fails
	differentPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate different key: %v", err)
	}
	differentSigner, err := ssh.NewSignerFromKey(differentPrivateKey)
	if err != nil {
		t.Fatalf("failed to create different signer: %v", err)
	}
	differentPubKey := differentSigner.PublicKey()

	err = callback("localhost:22", nil, differentPubKey)
	if err == nil {
		t.Fatal("expected connection with mismatching fingerprint to fail, but it succeeded")
	}
	if !strings.Contains(err.Error(), "HOST KEY VERIFICATION FAILED") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadOnlyEnforcement(t *testing.T) {
	setupTempHome(t)

	RegisterProfileForTest("readonly-profile", &HostProfile{
		Alias:    "readonly-profile",
		Host:     "localhost",
		Port:     22,
		User:     "test",
		ReadOnly: true,
	})
	defer DeleteProfileForTest("readonly-profile")

	ctx := context.Background()

	// 1. handleConnectAndExecute
	res, _, err := handleConnectAndExecute(ctx, nil, ConnectAndExecuteArgs{
		Profile:    "readonly-profile",
		Command:    "ls",
		GitWrapped: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "profile is configured as read-only; git-wrapped write operations are not permitted") {
		t.Errorf("expected read-only error from connect_and_execute, got: %+v", res)
	}

	// 2. handleSecureFileDelta (write)
	res, _, err = handleSecureFileDelta(ctx, nil, SecureFileDeltaArgs{
		Profile:    "readonly-profile",
		Operation:  "write",
		RemotePath: "/tmp/foo",
		Content:    "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "profile is configured as read-only; write operations are not permitted") {
		t.Errorf("expected read-only error from secure_file_delta (write), got: %+v", res)
	}

	// 3. handleSecureFileDelta (read - should not reject because of readonly, but fail on connection)
	res, _, err = handleSecureFileDelta(ctx, nil, SecureFileDeltaArgs{
		Profile:    "readonly-profile",
		Operation:  "read",
		RemotePath: "/tmp/foo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError && strings.Contains(res.Content[0].(*mcp.TextContent).Text, "profile is configured as read-only") {
		t.Errorf("unexpected read-only error from secure_file_delta (read), got: %+v", res)
	}

	// 4. handleSecureFileTransfer (upload)
	res, _, err = handleSecureFileTransfer(ctx, nil, SecureFileTransferArgs{
		Profile:    "readonly-profile",
		Direction:  "upload",
		LocalPath:  "/tmp/foo",
		RemotePath: "/tmp/foo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "profile is configured as read-only; write operations are not permitted") {
		t.Errorf("expected read-only error from secure_file_transfer (upload), got: %+v", res)
	}

	// 5. handleManageRemoteProcess (start)
	res, _, err = handleManageRemoteProcess(ctx, nil, ManageRemoteProcessArgs{
		Profile: "readonly-profile",
		Action:  "start",
		Command: "sleep 10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "profile is configured as read-only; write operations are not permitted") {
		t.Errorf("expected read-only error from manage_remote_process (start), got: %+v", res)
	}

	// 6. handleGitRollback
	res, _, err = handleGitRollback(ctx, nil, GitRollbackArgs{
		Profile: "readonly-profile",
		Workdir: "/tmp/gitrepo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "profile is configured as read-only; write operations are not permitted") {
		t.Errorf("expected read-only error from git_rollback, got: %+v", res)
	}
}

func TestRateLimiter(t *testing.T) {
	limit := 5
	profileKey := "user@localhost:22"

	for i := 0; i < 5; i++ {
		err := checkRateLimit("test-profile", profileKey, &limit)
		if err != nil {
			t.Fatalf("expected attempt %d to succeed, got: %v", i+1, err)
		}
	}

	err := checkRateLimit("test-profile", profileKey, &limit)
	if err == nil {
		t.Fatal("expected 6th attempt to fail, but it succeeded")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTimeout(t *testing.T) {
	listener, _ := startMockSSHServer(t)
	defer listener.Close()

	addr := listener.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	cfg := &ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password("test")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	client, err := ssh.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port), cfg)
	if err != nil {
		t.Fatalf("failed to dial mock SSH server: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = remoteExec(ctx, client, "sleep 10")
	if err == nil {
		t.Fatal("expected remoteExec to time out, but it succeeded")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAllowedPaths(t *testing.T) {
	allowed := []string{"/var/log", "/home/user/data/"}

	cases := []struct {
		path    string
		allowed bool
	}{
		{"/var/log", true},
		{"/var/log/syslog", true},
		{"/var/log/nginx/access.log", true},
		{"/var/log/../log/syslog", true},
		{"/var/log-malicious", false},
		{"/etc/passwd", false},
		{"../etc/passwd", false},
		{"/var/log/../../etc/passwd", false},
		{"/home/user/data/file.txt", true},
		{"/home/user/data/sub/file.txt", true},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			res := isPathAllowed(allowed, tc.path)
			if res != tc.allowed {
				t.Errorf("expected isPathAllowed(%q) = %v, got %v", tc.path, tc.allowed, res)
			}
		})
	}
}

func TestAuditLog(t *testing.T) {
	tempDir := setupTempHome(t)

	auditLog(AuditEntry{
		Profile:    "test-profile",
		Host:       "1.2.3.4",
		Tool:       "test-tool",
		Command:    "ls -la",
		ExitCode:   new(int),
		DurationMs: 123,
	})

	auditFilePath := filepath.Join(tempDir, ".ai-ssh-tools", "audit.log")
	data, err := os.ReadFile(auditFilePath)
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line in audit log, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to unmarshal JSON log: %v", err)
	}

	if entry["profile"] != "test-profile" || entry["host"] != "1.2.3.4" || entry["tool"] != "test-tool" || entry["command"] != "ls -la" {
		t.Errorf("unexpected logged fields: %+v", entry)
	}

	if _, exists := entry["password"]; exists {
		t.Error("audit log must not contain password")
	}
	if _, exists := entry["key_path"]; exists {
		t.Error("audit log must not contain key_path")
	}
	if _, exists := entry["content"]; exists {
		t.Error("audit log must not contain content")
	}
}

func TestProfileManagement(t *testing.T) {
	tempDir := t.TempDir()
	hostsPath := filepath.Join(tempDir, "ssh_hosts.json")
	t.Setenv("SSH_HOSTS_PATH", hostsPath)
	t.Setenv(profileWritesEnvVar, "1")

	// Ensure registry is loaded/initialized empty
	err := loadProfiles()
	if err != nil {
		t.Fatalf("loadProfiles failed: %v", err)
	}

	ctx := context.Background()

	// Call handleSaveSshProfile
	res, _, err := handleSaveSshProfile(ctx, nil, SaveSshProfileArgs{
		Alias: "test-dynamic-profile",
		Host:  "127.0.0.1",
		Port:  2222,
		User:  "test-user",
	})
	if err != nil {
		t.Fatalf("handleSaveSshProfile returned internal error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handleSaveSshProfile returned execution error: %+v", res.Content[0])
	}

	// Verify we can list it
	listRes, _, err := handleListProfiles(ctx, nil, ListProfilesArgs{})
	if err != nil {
		t.Fatalf("handleListProfiles failed: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("handleListProfiles returned error: %+v", listRes.Content[0])
	}

	outputText := listRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(outputText, "test-dynamic-profile") || !strings.Contains(outputText, "test-user") {
		t.Errorf("expected profile in list, got: %s", outputText)
	}
}

// TestProfileWritesDisabledByDefault verifies that save_ssh_profile refuses to
// write unless the operator explicitly opts in via the environment variable.
func TestProfileWritesDisabledByDefault(t *testing.T) {
	tempDir := t.TempDir()
	hostsPath := filepath.Join(tempDir, "ssh_hosts.json")
	t.Setenv("SSH_HOSTS_PATH", hostsPath)
	t.Setenv(profileWritesEnvVar, "")

	if err := loadProfiles(); err != nil {
		t.Fatalf("loadProfiles failed: %v", err)
	}

	res, _, err := handleSaveSshProfile(context.Background(), nil, SaveSshProfileArgs{
		Alias: "should-not-exist",
		Host:  "127.0.0.1",
		User:  "attacker",
	})
	if err != nil {
		t.Fatalf("handleSaveSshProfile returned internal error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected save_ssh_profile to be denied when profile writes are disabled")
	}
	if _, err := os.Stat(hostsPath); !os.IsNotExist(err) {
		t.Fatalf("expected no ssh_hosts.json to be written, stat err = %v", err)
	}

	profileRegistryMu.RLock()
	_, exists := profileRegistry["should-not-exist"]
	profileRegistryMu.RUnlock()
	if exists {
		t.Fatal("denied profile was still added to the registry")
	}
}

// TestProfileWritesEnabledFlag checks the accepted values of the opt-in variable.
func TestProfileWritesEnabledFlag(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"no":    false,
		"1":     true,
		"true":  true,
		"TRUE":  true,
		"yes":   true,
		"on":    true,
		" 1 ":   true,
	}
	for value, want := range cases {
		t.Setenv(profileWritesEnvVar, value)
		if got := profileWritesEnabled(); got != want {
			t.Errorf("profileWritesEnabled() with %q = %v, want %v", value, got, want)
		}
	}
}

// TestProfileWeakeningRejected covers every way an agent could try to loosen an
// existing profile's security posture through save_ssh_profile.
func TestProfileWeakeningRejected(t *testing.T) {
	rpm := func(n int) *int { return &n }

	base := &HostProfile{
		Alias:           "prod",
		Host:            "203.0.113.10",
		User:            "deploy",
		ReadOnly:        true,
		AllowedCommands: []string{"^git .*$", "^systemctl status .*$"},
		BlockedCommands: []string{".*rm -rf.*"},
		AllowedPaths:    []string{"/var/log"},
		RateLimitRPM:    rpm(30),
		HostKey:         "SHA256:abc",
	}

	weakened := map[string]*HostProfile{
		"clears readonly":          {Alias: "prod", ReadOnly: false, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30), HostKey: "SHA256:abc"},
		"empties allowed_commands": {Alias: "prod", ReadOnly: true, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30), HostKey: "SHA256:abc"},
		"widens allowed_commands":  {Alias: "prod", ReadOnly: true, AllowedCommands: append([]string{".*"}, base.AllowedCommands...), BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30), HostKey: "SHA256:abc"},
		"drops blocked_commands":   {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30), HostKey: "SHA256:abc"},
		"empties allowed_paths":    {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, RateLimitRPM: rpm(30), HostKey: "SHA256:abc"},
		"widens allowed_paths":     {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: []string{"/var/log", "/"}, RateLimitRPM: rpm(30), HostKey: "SHA256:abc"},
		"removes rate limit":       {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, HostKey: "SHA256:abc"},
		"raises rate limit":        {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(6000), HostKey: "SHA256:abc"},
		"unpins host key":          {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30)},
		"repins host key":          {Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30), HostKey: "SHA256:evil"},
	}

	for name, updated := range weakened {
		if err := checkProfileWeakening(base, updated); err == nil {
			t.Errorf("checkProfileWeakening allowed a weakening update that %s", name)
		}
	}
}

// TestProfileTighteningAllowed verifies that stricter updates still go through.
func TestProfileTighteningAllowed(t *testing.T) {
	rpm := func(n int) *int { return &n }

	base := &HostProfile{
		Alias:           "prod",
		AllowedCommands: []string{"^git .*$", "^systemctl status .*$"},
		BlockedCommands: []string{".*rm -rf.*"},
		AllowedPaths:    []string{"/var/log", "/srv/app"},
		RateLimitRPM:    rpm(30),
	}

	stricter := []*HostProfile{
		{Alias: "prod", ReadOnly: true, AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30)},
		{Alias: "prod", AllowedCommands: []string{"^git .*$"}, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30)},
		{Alias: "prod", AllowedCommands: base.AllowedCommands, BlockedCommands: []string{".*rm -rf.*", ".*curl.*"}, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(30)},
		{Alias: "prod", AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: []string{"/var/log"}, RateLimitRPM: rpm(30)},
		{Alias: "prod", AllowedCommands: base.AllowedCommands, BlockedCommands: base.BlockedCommands, AllowedPaths: base.AllowedPaths, RateLimitRPM: rpm(10)},
	}

	for i, updated := range stricter {
		if err := checkProfileWeakening(base, updated); err != nil {
			t.Errorf("checkProfileWeakening rejected stricter update %d: %v", i, err)
		}
	}

	// A brand-new alias has no prior posture to weaken.
	if err := checkProfileWeakening(nil, &HostProfile{Alias: "new"}); err != nil {
		t.Errorf("checkProfileWeakening rejected a new profile: %v", err)
	}
}

// TestSaveSshProfileRejectsWeakening exercises the guard through the MCP handler.
func TestSaveSshProfileRejectsWeakening(t *testing.T) {
	tempDir := t.TempDir()
	hostsPath := filepath.Join(tempDir, "ssh_hosts.json")
	t.Setenv("SSH_HOSTS_PATH", hostsPath)
	t.Setenv(profileWritesEnvVar, "1")

	seed := `[{"alias":"locked","host":"203.0.113.10","port":22,"user":"deploy","readonly":true,"blocked_commands":[".*rm -rf.*"]}]`
	if err := os.WriteFile(hostsPath, []byte(seed), 0600); err != nil {
		t.Fatalf("seeding ssh_hosts.json failed: %v", err)
	}
	if err := loadProfiles(); err != nil {
		t.Fatalf("loadProfiles failed: %v", err)
	}

	res, _, err := handleSaveSshProfile(context.Background(), nil, SaveSshProfileArgs{
		Alias:    "locked",
		Host:     "203.0.113.10",
		User:     "deploy",
		ReadOnly: false, // the bypass: flip readonly off and drop the blocklist
	})
	if err != nil {
		t.Fatalf("handleSaveSshProfile returned internal error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected save_ssh_profile to reject a weakening update")
	}

	profileRegistryMu.RLock()
	p := profileRegistry["locked"]
	profileRegistryMu.RUnlock()
	if p == nil {
		t.Fatal("profile disappeared from registry")
	}
	if !p.ReadOnly {
		t.Error("readonly was cleared despite the guard")
	}
	if len(p.BlockedCommands) != 1 {
		t.Errorf("blocked_commands were dropped: %v", p.BlockedCommands)
	}
}
