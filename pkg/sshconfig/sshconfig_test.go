package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchHostPattern(t *testing.T) {
	if !MatchHostPattern("*", "anything") {
		t.Errorf("expected * to match anything")
	}
	if !MatchHostPattern("prod-*", "prod-web-01") {
		t.Errorf("expected prod-* to match prod-web-01")
	}
	if MatchHostPattern("staging-*", "prod-web-01") {
		t.Errorf("expected staging-* not to match prod-web-01")
	}
}

func TestResolveSSHConfig(t *testing.T) {
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")

	os.Setenv("HOME", tempDir)
	os.Setenv("USERPROFILE", tempDir)
	defer func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldUserProfile)
	}()

	sshDir := filepath.Join(tempDir, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)

	config := `
Host my-vps
    HostName 188.245.72.2
    User root
    Port 22
    IdentityFile ~/.ssh/id_rsa
`
	_ = os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0600)

	cfg := ResolveSSHConfig("my-vps")
	if cfg == nil {
		t.Fatalf("expected resolved config, got nil")
	}
	if cfg.HostName != "188.245.72.2" {
		t.Errorf("expected HostName 188.245.72.2, got %s", cfg.HostName)
	}
	if cfg.User != "root" {
		t.Errorf("expected User root, got %s", cfg.User)
	}
	if !strings.HasSuffix(cfg.IdentityFile, "id_rsa") {
		t.Errorf("expected IdentityFile ending with id_rsa, got %s", cfg.IdentityFile)
	}
}
