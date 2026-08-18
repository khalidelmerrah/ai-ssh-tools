package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEntry struct {
	Timestamp        string `json:"ts"`
	Profile          string `json:"profile,omitempty"`
	Host             string `json:"host,omitempty"`
	Tool             string `json:"tool"`
	Command          string `json:"command,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	DurationMs       int64  `json:"duration_ms,omitempty"`
	BytesTransferred int64  `json:"bytes_transferred,omitempty"`
	Operation        string `json:"operation,omitempty"`
	Path             string `json:"path,omitempty"`
	ProcessID        string `json:"process_id,omitempty"`
	Action           string `json:"action,omitempty"`
	Status           string `json:"status,omitempty"`
	Error            string `json:"error,omitempty"`
}

var auditMu sync.Mutex

const maxAuditLogBytes = 10 * 1024 * 1024 // 10 MB

func auditLog(entry AuditEntry) {
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

	auditDir, err := appDataDir()
	if err != nil {
		log.Printf("[error] auditLog: failed to resolve application data directory: %v", err)
		return
	}
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		log.Printf("[error] auditLog: failed to create audit directory %s: %v", auditDir, err)
		return
	}

	auditFilePath := filepath.Join(auditDir, "audit.log")

	auditMu.Lock()
	defer auditMu.Unlock()

	// Rotate if file exceeds size limit.
	if info, err := os.Stat(auditFilePath); err == nil && info.Size() > maxAuditLogBytes {
		backupPath := auditFilePath + ".1"
		_ = os.Remove(backupPath)
		_ = os.Rename(auditFilePath, backupPath)
	}

	f, err := os.OpenFile(auditFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("[error] auditLog: failed to open audit log file %s: %v", auditFilePath, err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("[error] auditLog: failed to serialize entry: %v", err)
		return
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		log.Printf("[error] auditLog: failed to write to audit log: %v", err)
	}
}
