package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type poolEntry struct {
	client    *ssh.Client
	keepalive *time.Ticker
	mu        sync.Mutex
}

var (
	sessionPool   = map[string]*poolEntry{}
	sessionPoolMu sync.RWMutex
)

// poolKey returns the cache key for a connection.
func poolKey(user, host string, port int) string {
	return fmt.Sprintf("%s@%s:%d", user, host, port)
}

// TOFU registry
var knownHostsMu sync.Mutex

func getTOFUHostKeyCallback(userHostPort string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home: %w", err)
		}
		tofuDir := filepath.Join(home, ".ai-ssh-tools")
		tofuPath := filepath.Join(tofuDir, "known_hosts.json")

		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()

		if err := os.MkdirAll(tofuDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", tofuDir, err)
		}

		hosts := make(map[string]string)
		data, err := os.ReadFile(tofuPath)
		if err == nil {
			_ = json.Unmarshal(data, &hosts)
		}

		saved, exists := hosts[userHostPort]
		if !exists {
			hosts[userHostPort] = fingerprint
			updatedData, err := json.MarshalIndent(hosts, "", "  ")
			if err == nil {
				_ = os.WriteFile(tofuPath, updatedData, 0600)
			}
			log.Printf("[WARN] New host fingerprint saved for %s: %s. Verify this is correct.", userHostPort, fingerprint)
			return nil
		}

		if saved != fingerprint {
			return fmt.Errorf("HOST KEY VERIFICATION FAILED FOR %s. Expected fingerprint: %s. Got: %s. Potential Man-in-the-Middle attack or host key changed.", userHostPort, saved, fingerprint)
		}

		return nil
	}
}

// Circuit Breaker registry
type CircuitBreaker struct {
	mu             sync.Mutex
	consecFailures int
	cooldownUntil  time.Time
}

var (
	circuitBreakers   = map[string]*CircuitBreaker{}
	circuitBreakersMu sync.Mutex
)

func getCircuitBreaker(key string) *CircuitBreaker {
	circuitBreakersMu.Lock()
	defer circuitBreakersMu.Unlock()
	cb, exists := circuitBreakers[key]
	if !exists {
		cb = &CircuitBreaker{}
		circuitBreakers[key] = cb
	}
	return cb
}

// getOrConnect returns an existing cached *ssh.Client or dials a new one.
func getOrConnect(profile *HostProfile) (*ssh.Client, error) {
	key := poolKey(profile.User, profile.Host, profile.Port)

	// Circuit Breaker Check
	cb := getCircuitBreaker(key)
	cb.mu.Lock()
	if time.Now().Before(cb.cooldownUntil) {
		cb.mu.Unlock()
		err := fmt.Errorf("circuit breaker open for %s: too many consecutive failures, retry after %s", profile.Host, cb.cooldownUntil.Format(time.RFC3339))
		auditLog(AuditEntry{
			Profile: profile.Alias,
			Host:    profile.Host,
			Tool:    "connection_attempt",
			Status:  "circuit_breaker_open",
			Error:   err.Error(),
		})
		return nil, err
	}
	cb.mu.Unlock()

	sessionPoolMu.RLock()
	entry, ok := sessionPool[key]
	sessionPoolMu.RUnlock()

	if ok {
		entry.mu.Lock()
		defer entry.mu.Unlock()
		// Validate the connection is still alive via a cheap keepalive ping.
		if _, _, err := entry.client.SendRequest("keepalive@openssh.com", true, nil); err == nil {
			return entry.client, nil
		}
		// Stale — fall through to re-dial.
		entry.client.Close()
		entry.keepalive.Stop()
	}

	// Build auth methods — credentials never leave the server process.
	var authMethods []ssh.AuthMethod

	if profile.KeyPath != "" {
		expanded := expandTilde(profile.KeyPath)
		keyBytes, err := os.ReadFile(expanded)
		if err != nil {
			return nil, fmt.Errorf("reading private key %q: %w", expanded, err)
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("parsing private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if profile.Password != "" {
		authMethods = append(authMethods, ssh.Password(profile.Password))
	}

	if len(authMethods) == 0 {
		// Try default ~/.ssh/id_rsa / id_ed25519 as a fallback.
		for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
			p := expandTilde("~/.ssh/" + name)
			if data, err := os.ReadFile(p); err == nil {
				if signer, err := ssh.ParsePrivateKey(data); err == nil {
					authMethods = append(authMethods, ssh.PublicKeys(signer))
					break
				}
			}
		}
	}

	var hostKeyCallback ssh.HostKeyCallback
	if profile.HostKey != "" {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			fpSHA256 := ssh.FingerprintSHA256(key)
			fpMD5 := ssh.FingerprintLegacyMD5(key)
			expected := strings.TrimSpace(profile.HostKey)

			if expected == fpSHA256 || expected == fpMD5 {
				return nil
			}

			// Match without prefixes
			if strings.TrimPrefix(expected, "SHA256:") == strings.TrimPrefix(fpSHA256, "SHA256:") {
				return nil
			}
			if strings.TrimPrefix(expected, "MD5:") == strings.TrimPrefix(fpMD5, "MD5:") {
				return nil
			}

			marshaled := key.Marshal()
			base64Str := base64.StdEncoding.EncodeToString(marshaled)
			if expected == base64Str || strings.Contains(expected, base64Str) {
				return nil
			}

			parsedExpectedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(expected))
			if err == nil {
				if bytes.Equal(parsedExpectedKey.Marshal(), marshaled) {
					return nil
				}
			}

			return fmt.Errorf("SSH host key verification failed. Expected: %q. Server presented fingerprint SHA256: %s, MD5: %s", expected, fpSHA256, fpMD5)
		}
	} else {
		hostKeyCallback = getTOFUHostKeyCallback(key)
	}

	cfg := &ssh.ClientConfig{
		User:            profile.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", profile.Host, profile.Port)

	auditLog(AuditEntry{
		Profile: profile.Alias,
		Host:    profile.Host,
		Tool:    "connection_attempt",
		Status:  "attempting",
	})

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		cb.mu.Lock()
		cb.consecFailures++
		if cb.consecFailures >= 5 {
			cb.cooldownUntil = time.Now().Add(60 * time.Second)
		}
		cb.mu.Unlock()

		auditLog(AuditEntry{
			Profile: profile.Alias,
			Host:    profile.Host,
			Tool:    "connection_attempt",
			Status:  "failure",
			Error:   err.Error(),
		})
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	// Reset consec failures
	cb.mu.Lock()
	cb.consecFailures = 0
	cb.mu.Unlock()

	auditLog(AuditEntry{
		Profile: profile.Alias,
		Host:    profile.Host,
		Tool:    "connection_attempt",
		Status:  "success",
	})

	// Register keepalive goroutine (30-second interval).
	ticker := time.NewTicker(30 * time.Second)
	newEntry := &poolEntry{client: client, keepalive: ticker}
	go func() {
		for range ticker.C {
			newEntry.mu.Lock()
			if _, _, err := newEntry.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				log.Printf("[warn] keepalive failed for %s; connection may be stale", key)
			}
			newEntry.mu.Unlock()
		}
	}()

	sessionPoolMu.Lock()
	sessionPool[key] = newEntry
	sessionPoolMu.Unlock()

	return client, nil
}
