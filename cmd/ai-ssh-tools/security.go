package main

import (
	"fmt"
	"regexp"
	"strings"
)

// dangerousTokens matches shell chaining operators and unsafe backtick usage.
var dangerousTokens = regexp.MustCompile(`(;|&&|\|\||` + "`" + `|\$\()`)

// sanitiseCommand validates a command string for safety.
// Returns a cleaned command and any violation description.
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
