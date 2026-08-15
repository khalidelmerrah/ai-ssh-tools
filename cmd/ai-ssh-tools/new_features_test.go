package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmartTruncate(t *testing.T) {
	// Small output should remain untouched
	small := "Hello, World!\nSecond line"
	if res := smartTruncate(small, 1000, 10); res != small {
		t.Errorf("expected %q, got %q", small, res)
	}

	// Large line count should truncate
	var manyLines []string
	for i := 0; i < 500; i++ {
		manyLines = append(manyLines, "line content")
	}
	largeLines := strings.Join(manyLines, "\n")
	res := smartTruncate(largeLines, 100*1024, 50)
	if !strings.Contains(res, "TRUNCATED") {
		t.Errorf("expected truncation marker in output")
	}
	if !strings.HasPrefix(res, "line content") {
		t.Errorf("expected head lines preserved")
	}
	if !strings.HasSuffix(res, "line content") {
		t.Errorf("expected tail lines preserved")
	}

	// Large byte size should truncate
	largeBytes := strings.Repeat("A", 50*1024)
	resBytes := smartTruncate(largeBytes, 10*1024, 1000)
	if !strings.Contains(resBytes, "TRUNCATED") {
		t.Errorf("expected byte truncation marker")
	}
}

func TestSSHConfigParsing(t *testing.T) {
	tempHome := setupTempHome(t)
	sshDir := filepath.Join(tempHome, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)

	configContent := `
Host production-box
    HostName 192.168.10.55
    User devops
    Port 2200
    IdentityFile ~/.ssh/prod_rsa

Host staging-*
    User staging_user
`
	configPath := filepath.Join(sshDir, "config")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to write mock ssh config: %v", err)
	}

	cfg := resolveSSHConfig("production-box")
	if cfg == nil {
		t.Fatalf("expected config for production-box, got nil")
	}
	if cfg.HostName != "192.168.10.55" {
		t.Errorf("expected HostName 192.168.10.55, got %s", cfg.HostName)
	}
	if cfg.User != "devops" {
		t.Errorf("expected User devops, got %s", cfg.User)
	}
	if cfg.Port != 2200 {
		t.Errorf("expected Port 2200, got %d", cfg.Port)
	}
	if !strings.HasSuffix(cfg.IdentityFile, "prod_rsa") {
		t.Errorf("expected IdentityFile with prod_rsa, got %s", cfg.IdentityFile)
	}

	// Test profile resolution via ~/.ssh/config
	prof, err := resolveProfile("production-box", "", "", 0)
	if err != nil {
		t.Fatalf("resolveProfile failed for ssh config host: %v", err)
	}
	if prof.Host != "192.168.10.55" || prof.User != "devops" || prof.Port != 2200 {
		t.Errorf("unexpected resolved profile: %+v", prof)
	}
}
