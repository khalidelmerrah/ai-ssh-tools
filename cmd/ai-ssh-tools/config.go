package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// HostProfile describes a named SSH target loaded from ssh_hosts.json.
type HostProfile struct {
	Alias           string   `json:"alias"`
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	User            string   `json:"user"`
	KeyPath         string   `json:"key_path"`    // path to private key; never exposed to LLM
	Password        string   `json:"password"`    // optional, never exposed to LLM
	UseAgent        bool     `json:"use_agent"`   // whether to use the SSH agent socket
	GitEnabled      bool     `json:"git_enabled"` // wrap executions in pre/post git snapshots
	AllowedCommands []string `json:"allowed_commands"`
	BlockedCommands []string `json:"blocked_commands"`
	HostKey         string   `json:"host_key"`
	ReadOnly        bool     `json:"readonly"`
	RateLimitRPM    *int     `json:"rate_limit_rpm"`
	AllowedPaths    []string `json:"allowed_paths"`
	allowedRegexes  []*regexp.Regexp
	blockedRegexes  []*regexp.Regexp
}

// profileRegistry holds the loaded host profiles, keyed by alias.
var (
	profileRegistry   = map[string]*HostProfile{}
	profileRegistryMu sync.RWMutex
)

// loadProfiles reads ssh_hosts.json from the same directory as the binary.
func loadProfiles() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(exe), "ssh_hosts.json")

	// Allow override via environment variable.
	if env := os.Getenv("SSH_HOSTS_PATH"); env != "" {
		path = env
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[warn] ssh_hosts.json not found at %s; profile-based connections disabled", path)
			profileRegistryMu.Lock()
			profileRegistry = map[string]*HostProfile{}
			profileRegistryMu.Unlock()
			return nil
		}
		return fmt.Errorf("reading ssh_hosts.json: %w", err)
	}

	var profiles []HostProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return fmt.Errorf("parsing ssh_hosts.json: %w", err)
	}

	tempRegistry := map[string]*HostProfile{}
	for i := range profiles {
		p := &profiles[i]
		if p.Port == 0 {
			p.Port = 22
		}
		for _, pat := range p.AllowedCommands {
			re, err := regexp.Compile(pat)
			if err != nil {
				return fmt.Errorf("invalid allowed_command regex %q for alias %s: %w", pat, p.Alias, err)
			}
			p.allowedRegexes = append(p.allowedRegexes, re)
		}
		for _, pat := range p.BlockedCommands {
			re, err := regexp.Compile(pat)
			if err != nil {
				return fmt.Errorf("invalid blocked_command regex %q for alias %s: %w", pat, p.Alias, err)
			}
			p.blockedRegexes = append(p.blockedRegexes, re)
		}
		tempRegistry[p.Alias] = p
	}

	profileRegistryMu.Lock()
	profileRegistry = tempRegistry
	profileRegistryMu.Unlock()

	log.Printf("[info] loaded %d SSH profile(s)", len(tempRegistry))
	return nil
}

// saveProfile writes/updates a profile in ssh_hosts.json and reloads the registry.
func saveProfile(p HostProfile) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(exe), "ssh_hosts.json")

	if env := os.Getenv("SSH_HOSTS_PATH"); env != "" {
		path = env
	}

	var profiles []HostProfile
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &profiles); err != nil {
			if len(strings.TrimSpace(string(data))) > 0 {
				return fmt.Errorf("reading existing ssh_hosts.json: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading ssh_hosts.json: %w", err)
	}

	found := false
	for i, existing := range profiles {
		if existing.Alias == p.Alias {
			profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		profiles = append(profiles, p)
	}

	updatedData, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profiles: %w", err)
	}

	if err := os.WriteFile(path, updatedData, 0600); err != nil {
		return fmt.Errorf("writing ssh_hosts.json: %w", err)
	}

	return loadProfiles()
}

// RegisterProfileForTest registers a profile for testing purposes.
func RegisterProfileForTest(alias string, p *HostProfile) {
	profileRegistryMu.Lock()
	defer profileRegistryMu.Unlock()
	profileRegistry[alias] = p
}

// DeleteProfileForTest deletes a profile after testing.
func DeleteProfileForTest(alias string) {
	profileRegistryMu.Lock()
	defer profileRegistryMu.Unlock()
	delete(profileRegistry, alias)
}

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
