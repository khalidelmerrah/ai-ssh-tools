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
	if match := dangerousTokens.FindString(cmd); match != "" {
		return "", fmt.Errorf(
			"command rejected: contains disallowed token %q — use a single atomic command per call",
			match,
		)
	}
	// Strip leading/trailing whitespace.
	return strings.TrimSpace(cmd), nil
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
