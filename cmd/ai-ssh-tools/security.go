package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// dangerousTokens matches shell chaining operators and unsafe backtick usage.
var dangerousTokens = regexp.MustCompile(`(;|&&|\|\||` + "`" + `|\$\()`)

// sanitiseCommand validates a command string for safety.
// Returns a cleaned command and any violation description.
//
// SCOPE — this is an anti-chaining filter, NOT a harm filter. It rejects
// compound payloads (`;`, `&&`, `||`, backticks, `$()`, newlines) so that a
// single injected argument cannot smuggle in a second command. It deliberately
// does NOT reject pipes (`|`), redirections (`>`, `>>`, `<`), background (`&`),
// variable expansion (`$VAR`), globs, or destructive single commands such as
// `rm -rf /`. Restricting *what* a profile may run is the job of the
// per-profile allowed_commands whitelist; treat that as the real boundary.
func sanitiseCommand(cmd string) (string, error) {
	trimmed := strings.TrimSpace(cmd)
	if strings.ContainsAny(trimmed, "\r\n") {
		return "", fmt.Errorf("command rejected: contains newline character(s) — submit only a single atomic command per call")
	}
	if match := dangerousTokens.FindString(trimmed); match != "" {
		return "", fmt.Errorf(
			"command rejected: contains disallowed token %q — use a single atomic command per call",
			match,
		)
	}
	return trimmed, nil
}

// validateCommandPolicy sanitises a command and checks it against the profile's allowed/blocked lists.
func validateCommandPolicy(profile *HostProfile, cmd string) (string, error) {
	cleanCmd, err := sanitiseCommand(cmd)
	if err != nil {
		return "", err
	}
	if cleanCmd == "" {
		return "", fmt.Errorf("command must not be empty")
	}
	if profile != nil {
		if len(profile.allowedRegexes) > 0 {
			matched := false
			for _, re := range profile.allowedRegexes {
				if re.MatchString(cleanCmd) {
					matched = true
					break
				}
			}
			if !matched {
				return "", fmt.Errorf("command policy error: command %q is not allowed by whitelist", cleanCmd)
			}
		}
		for _, re := range profile.blockedRegexes {
			if re.MatchString(cleanCmd) {
				return "", fmt.Errorf("command policy error: command %q is blocked by blacklist", cleanCmd)
			}
		}
	}
	return cleanCmd, nil
}

// profileWritesEnvVar gates the save_ssh_profile MCP tool.
const profileWritesEnvVar = "AI_SSH_ALLOW_PROFILE_WRITES"

// profileWritesEnabled reports whether profile writes over MCP are permitted.
//
// Profile writes are denied by default: every other guardrail in this server
// (readonly, allowed_commands, blocked_commands, allowed_paths, rate limits,
// pinned host keys) lives in the profile, so an agent able to rewrite profiles
// can disarm all of them in a single call. The CLI path is unaffected — a human
// at a terminal is already trusted.
func profileWritesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(profileWritesEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// checkProfileWeakening rejects an update that loosens an existing profile's
// security posture. Applied even when profile writes are enabled, so that
// turning writes on for convenience does not also hand over the guardrails.
// A profile may always be made stricter.
func checkProfileWeakening(old, updated *HostProfile) error {
	if old == nil {
		return nil
	}

	if old.ReadOnly && !updated.ReadOnly {
		return fmt.Errorf("profile %q is read-only; clearing readonly over MCP is not permitted", old.Alias)
	}

	// allowed_commands and allowed_paths are whitelists: emptying one removes the
	// restriction entirely, and adding an entry widens it. Both are weakenings.
	if err := checkWhitelistNotWidened("allowed_commands", old.Alias, old.AllowedCommands, updated.AllowedCommands); err != nil {
		return err
	}
	if err := checkWhitelistNotWidened("allowed_paths", old.Alias, old.AllowedPaths, updated.AllowedPaths); err != nil {
		return err
	}

	// blocked_commands is a blacklist: dropping an entry is a weakening.
	updatedBlocked := make(map[string]bool, len(updated.BlockedCommands))
	for _, b := range updated.BlockedCommands {
		updatedBlocked[b] = true
	}
	for _, b := range old.BlockedCommands {
		if !updatedBlocked[b] {
			return fmt.Errorf(
				"profile %q: removing blocked_commands entry %q over MCP is not permitted",
				old.Alias, b,
			)
		}
	}

	if old.RateLimitRPM != nil {
		if updated.RateLimitRPM == nil {
			return fmt.Errorf("profile %q: removing rate_limit_rpm over MCP is not permitted", old.Alias)
		}
		if *updated.RateLimitRPM > *old.RateLimitRPM {
			return fmt.Errorf(
				"profile %q: raising rate_limit_rpm from %d to %d over MCP is not permitted",
				old.Alias, *old.RateLimitRPM, *updated.RateLimitRPM,
			)
		}
	}

	if old.HostKey != "" && updated.HostKey != old.HostKey {
		return fmt.Errorf("profile %q: changing the pinned host_key over MCP is not permitted", old.Alias)
	}

	return nil
}

// checkWhitelistNotWidened fails if a non-empty whitelist is emptied or gains entries.
func checkWhitelistNotWidened(field, alias string, old, updated []string) error {
	if len(old) == 0 {
		return nil
	}
	if len(updated) == 0 {
		return fmt.Errorf("profile %q: clearing the %s whitelist over MCP is not permitted", alias, field)
	}
	allowed := make(map[string]bool, len(old))
	for _, o := range old {
		allowed[o] = true
	}
	for _, u := range updated {
		if !allowed[u] {
			return fmt.Errorf(
				"profile %q: adding %s entry %q over MCP is not permitted (whitelists may only be narrowed)",
				alias, field, u,
			)
		}
	}
	return nil
}

// shellQuote safely quotes a string for use as a literal argument in a shell command.
// It wraps the value in single quotes and escapes any embedded single quotes,
// preventing shell injection via interpolated paths, service names, etc.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
