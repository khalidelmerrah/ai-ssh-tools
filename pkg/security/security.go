package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DangerousTokens matches shell chaining operators and unsafe subshell usage.
var DangerousTokens = regexp.MustCompile(`(;|&&|\|\||` + "`" + `|\$\()`)

// SanitiseCommand validates a command string for safety.
func SanitiseCommand(cmd string) (string, error) {
	trimmed := strings.TrimSpace(cmd)
	if strings.ContainsAny(trimmed, "\r\n") {
		return "", fmt.Errorf("command rejected: contains newline character(s) — submit only a single atomic command per call")
	}
	if match := DangerousTokens.FindString(trimmed); match != "" {
		return "", fmt.Errorf(
			"command rejected: contains disallowed token %q — use a single atomic command per call",
			match,
		)
	}
	return trimmed, nil
}

// ValidatePolicy validates a command against allowed and blocked regex lists.
func ValidatePolicy(cmd string, allowedRegexes, blockedRegexes []*regexp.Regexp) (string, error) {
	cleanCmd, err := SanitiseCommand(cmd)
	if err != nil {
		return "", err
	}
	if cleanCmd == "" {
		return "", fmt.Errorf("command must not be empty")
	}

	if len(allowedRegexes) > 0 {
		matched := false
		for _, re := range allowedRegexes {
			if re.MatchString(cleanCmd) {
				matched = true
				break
			}
		}
		if !matched {
			return "", fmt.Errorf("command policy error: command %q is not allowed by whitelist", cleanCmd)
		}
	}

	for _, re := range blockedRegexes {
		if re.MatchString(cleanCmd) {
			return "", fmt.Errorf("command policy error: command %q is blocked by blacklist", cleanCmd)
		}
	}

	return cleanCmd, nil
}

// SmartTruncate truncates output exceeding maxBytes or maxLines, returning a clean preview.
func SmartTruncate(output string, maxBytes, maxLines int) string {
	if maxBytes <= 0 {
		maxBytes = 40 * 1024 // 40 KB default
	}
	if maxLines <= 0 {
		maxLines = 400
	}

	if len(output) <= maxBytes && strings.Count(output, "\n") <= maxLines {
		return output
	}

	lines := strings.Split(output, "\n")
	totalLines := len(lines)
	totalBytes := len(output)

	headCount := 30
	tailCount := 100

	if totalLines <= headCount+tailCount {
		if len(output) > maxBytes {
			return output[:maxBytes] + fmt.Sprintf("\n\n... [TRUNCATED %d bytes out of %d total bytes] ...", totalBytes-maxBytes, totalBytes)
		}
		return output
	}

	head := strings.Join(lines[:headCount], "\n")
	tail := strings.Join(lines[totalLines-tailCount:], "\n")
	truncatedLines := totalLines - (headCount + tailCount)

	return fmt.Sprintf("%s\n\n... [TRUNCATED %d lines / %d bytes] ...\n\n%s", head, truncatedLines, totalBytes, tail)
}

// IsPathAllowed checks if target path is within allowed directory boundaries.
func IsPathAllowed(allowedPaths []string, remotePath string) bool {
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

// AuditEntry defines structured audit log items.
type AuditEntry struct {
	Timestamp  string `json:"timestamp"`
	Profile    string `json:"profile,omitempty"`
	Host       string `json:"host,omitempty"`
	Tool       string `json:"tool"`
	Operation  string `json:"operation,omitempty"`
	Path       string `json:"path,omitempty"`
	Command    string `json:"command,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	DurationMs int64  `json:"duration_ms"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
}

var auditMu sync.Mutex

// AuditLog writes an audit entry in JSON format.
func AuditLog(entry AuditEntry) {
	auditMu.Lock()
	defer auditMu.Unlock()

	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	auditDir := filepath.Join(home, ".ai-ssh-tools")
	auditPath := filepath.Join(auditDir, "audit.log")

	_ = os.MkdirAll(auditDir, 0755)

	f, err := os.OpenFile(auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
}

// RateLimiter manages sliding window rate limits.
type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

var (
	rateLimits   = map[string]*rateLimitEntry{}
	rateLimitsMu sync.Mutex
)

// CheckRateLimit validates RPM limits for a connection key.
func CheckRateLimit(alias, key string, customRPM *int) error {
	limit := 60
	if customRPM != nil {
		if *customRPM <= 0 {
			return nil
		}
		limit = *customRPM
	}

	rateLimitsMu.Lock()
	defer rateLimitsMu.Unlock()

	now := time.Now()
	entry, exists := rateLimits[key]
	if !exists || now.After(entry.resetTime) {
		rateLimits[key] = &rateLimitEntry{
			count:     1,
			resetTime: now.Add(time.Minute),
		}
		return nil
	}

	if entry.count >= limit {
		err := fmt.Errorf("rate limit exceeded for %s (%s): max %d requests per minute", alias, key, limit)
		AuditLog(AuditEntry{
			Profile: alias,
			Tool:    "rate_limiter",
			Status:  "rate_limit_exceeded",
			Error:   err.Error(),
		})
		return err
	}

	entry.count++
	return nil
}
