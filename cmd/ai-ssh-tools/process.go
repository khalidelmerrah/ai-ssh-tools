package main

import (
	"fmt"
	"time"
)

func isValidProcessID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func generateProcessID() string {
	return fmt.Sprintf("proc_%d", time.Now().UnixNano())
}
