package sshconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SSHConfigHost holds settings resolved from ~/.ssh/config.
type SSHConfigHost struct {
	HostName     string
	User         string
	Port         int
	IdentityFile string
}

// MatchHostPattern matches standard SSH host patterns including wildcards.
func MatchHostPattern(pattern, host string) bool {
	if pattern == "*" || pattern == host {
		return true
	}
	matched, err := filepath.Match(pattern, host)
	return err == nil && matched
}

// ResolveSSHConfig reads ~/.ssh/config and matches host aliases.
func ResolveSSHConfig(alias string) *SSHConfigHost {
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
				if MatchHostPattern(pat, alias) {
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
					matchedConfig.IdentityFile = ExpandTilde(val)
				}
			}
		}
	}

	return matchedConfig
}

// ExpandTilde replaces a leading ~ with the user's home directory.
func ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
