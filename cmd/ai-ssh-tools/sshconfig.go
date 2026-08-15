package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHConfigHost holds settings resolved from ~/.ssh/config.
type SSHConfigHost struct {
	HostName     string
	User         string
	Port         int
	IdentityFile string
}

func matchHostPattern(pattern, host string) bool {
	if pattern == "*" || pattern == host {
		return true
	}
	matched, err := filepath.Match(pattern, host)
	return err == nil && matched
}

// resolveSSHConfig reads ~/.ssh/config and matches host aliases.
func resolveSSHConfig(alias string) *SSHConfigHost {
	if alias == "" {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	configPath := filepath.Join(home, ".ssh", "config")

	f, err := os.Open(configPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	currentSectionMatches := false
	var matchedConfig *SSHConfigHost

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		val := strings.Join(parts[1:], " ")

		if key == "host" {
			currentSectionMatches = false
			for _, pat := range parts[1:] {
				if matchHostPattern(pat, alias) {
					currentSectionMatches = true
					if matchedConfig == nil {
						matchedConfig = &SSHConfigHost{
							HostName: alias,
							Port:     22,
						}
					}
					break
				}
			}
			continue
		}

		if currentSectionMatches && matchedConfig != nil {
			switch key {
			case "hostname":
				if matchedConfig.HostName == alias || matchedConfig.HostName == "" {
					matchedConfig.HostName = val
				}
			case "user":
				if matchedConfig.User == "" {
					matchedConfig.User = val
				}
			case "port":
				if matchedConfig.Port == 22 || matchedConfig.Port == 0 {
					if p, err := strconv.Atoi(val); err == nil && p > 0 {
						matchedConfig.Port = p
					}
				}
			case "identityfile":
				if matchedConfig.IdentityFile == "" {
					matchedConfig.IdentityFile = expandTilde(val)
				}
			}
		}
	}

	return matchedConfig
}

// getSSHAgentAuthMethod connects to the running SSH agent (via SSH_AUTH_SOCK or Windows OpenSSH agent pipe).
func getSSHAgentAuthMethod() ssh.AuthMethod {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		// Check standard Windows OpenSSH named pipe
		sock = `\\.\pipe\openssh-ssh-agent`
	}

	var conn net.Conn
	var err error

	if strings.HasPrefix(sock, `\\.\pipe\`) {
		conn, err = dialWindowsNamedPipe(sock)
	} else {
		conn, err = net.Dial("unix", sock)
	}

	if err != nil {
		return nil
	}

	agentClient := agent.NewClient(conn)
	signers, err := agentClient.Signers()
	if err != nil || len(signers) == 0 {
		conn.Close()
		return nil
	}

	return ssh.PublicKeysCallback(agentClient.Signers)
}

// smartTruncate truncates output exceeding maxBytes or maxLines, returning a clean preview.
func smartTruncate(output string, maxBytes, maxLines int) string {
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
