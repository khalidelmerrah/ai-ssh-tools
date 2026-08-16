package vitals

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// SystemVitals holds formatted system diagnostics.
type SystemVitals struct {
	OSName        string       `json:"os_name"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	LoadAverages  []float64    `json:"load_averages"`
	MemoryBytes   MemoryVitals `json:"memory_bytes"`
	Disks         []DiskVitals `json:"disks"`
}

// MemoryVitals holds memory statistics.
type MemoryVitals struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	PercentUsed float64 `json:"percent_used"`
}

// DiskVitals holds disk mount statistics.
type DiskVitals struct {
	Mount       string  `json:"mount"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	PercentUsed float64 `json:"percent_used"`
}

// FetchVitals collects remote system vitals from Linux procfs and coreutils.
func FetchVitals(ctx context.Context, client *ssh.Client) (*SystemVitals, error) {
	osRes, _ := runQuickCmd(ctx, client, "cat /etc/os-release")
	osName := "Linux (Unknown)"
	if osRes != "" {
		osName = ParseOSName(osRes)
	}

	uptimeRes, _ := runQuickCmd(ctx, client, "cat /proc/uptime")
	var uptime int64
	if uptimeRes != "" {
		uptime = ParseUptime(uptimeRes)
	}

	loadRes, _ := runQuickCmd(ctx, client, "cat /proc/loadavg")
	var loads []float64
	if loadRes != "" {
		loads = ParseLoadAverages(loadRes)
	}

	memRes, _ := runQuickCmd(ctx, client, "free -b")
	var mem MemoryVitals
	if memRes != "" {
		mem = ParseMemoryBytes(memRes)
	}

	diskRes, _ := runQuickCmd(ctx, client, "df -B1")
	var disks []DiskVitals
	if diskRes != "" {
		disks = ParseDisks(diskRes)
	}

	return &SystemVitals{
		OSName:        osName,
		UptimeSeconds: uptime,
		LoadAverages:  loads,
		MemoryBytes:   mem,
		Disks:         disks,
	}, nil
}

func runQuickCmd(ctx context.Context, client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout

	done := make(chan error, 1)
	go func() {
		done <- sess.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		sess.Close()
		return "", ctx.Err()
	case err := <-done:
		if err != nil {
			return "", err
		}
		return stdout.String(), nil
	}
}

// ParseMemoryBytes parses the output of `free -b`.
func ParseMemoryBytes(freeOutput string) MemoryVitals {
	lines := strings.Split(freeOutput, "\n")
	var vitals MemoryVitals
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasPrefix(fields[0], "Mem:") {
			total, _ := strconv.ParseUint(fields[1], 10, 64)
			used, _ := strconv.ParseUint(fields[2], 10, 64)
			free, _ := strconv.ParseUint(fields[3], 10, 64)
			vitals.Total = total
			vitals.Used = used
			vitals.Free = free
			if total > 0 {
				vitals.PercentUsed = (float64(used) / float64(total)) * 100.0
			}
			break
		}
	}
	return vitals
}

// ParseDisks parses the output of `df -B1`.
func ParseDisks(dfOutput string) []DiskVitals {
	lines := strings.Split(dfOutput, "\n")
	var disks []DiskVitals
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 6 && !strings.HasPrefix(fields[0], "Filesystem") {
			total, _ := strconv.ParseUint(fields[1], 10, 64)
			used, _ := strconv.ParseUint(fields[2], 10, 64)
			free, _ := strconv.ParseUint(fields[3], 10, 64)
			mount := fields[5]

			if strings.HasPrefix(mount, "/snap") || strings.HasPrefix(mount, "/run") || strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/sys") {
				continue
			}

			var percent float64
			if total > 0 {
				percent = (float64(used) / float64(total)) * 100.0
			}

			disks = append(disks, DiskVitals{
				Mount:       mount,
				TotalBytes:  total,
				UsedBytes:   used,
				FreeBytes:   free,
				PercentUsed: percent,
			})
		}
	}
	return disks
}

// ParseLoadAverages parses the output of `cat /proc/loadavg`.
func ParseLoadAverages(loadavgOutput string) []float64 {
	fields := strings.Fields(loadavgOutput)
	var loads []float64
	for i := 0; i < 3 && i < len(fields); i++ {
		val, err := strconv.ParseFloat(fields[i], 64)
		if err == nil {
			loads = append(loads, val)
		}
	}
	return loads
}

// ParseUptime parses the output of `cat /proc/uptime`.
func ParseUptime(uptimeOutput string) int64 {
	fields := strings.Fields(uptimeOutput)
	if len(fields) > 0 {
		val, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			return int64(val)
		}
	}
	return 0
}

// ParseOSName parses `PRETTY_NAME` or `NAME` from `/etc/os-release`.
func ParseOSName(osReleaseOutput string) string {
	lines := strings.Split(osReleaseOutput, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"'`)
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "NAME="), `"'`)
		}
	}
	return "Linux (Unknown)"
}

// FormatVitals formats a SystemVitals struct into a human-readable string.
func FormatVitals(alias string, v *SystemVitals) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- System Vitals: %s (%s) ---\n", alias, v.OSName)
	fmt.Fprintf(&sb, "Uptime:       %d seconds\n", v.UptimeSeconds)
	if len(v.LoadAverages) >= 3 {
		fmt.Fprintf(&sb, "Load Avg:     1m: %.2f | 5m: %.2f | 15m: %.2f\n", v.LoadAverages[0], v.LoadAverages[1], v.LoadAverages[2])
	}
	usedMB := v.MemoryBytes.Used / (1024 * 1024)
	totalMB := v.MemoryBytes.Total / (1024 * 1024)
	fmt.Fprintf(&sb, "Memory:       %d MB / %d MB (%.1f%% used)\n", usedMB, totalMB, v.MemoryBytes.PercentUsed)

	if len(v.Disks) > 0 {
		sb.WriteString("Disk Mounts:\n")
		for _, d := range v.Disks {
			usedGB := float64(d.UsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(d.TotalBytes) / (1024 * 1024 * 1024)
			fmt.Fprintf(&sb, "  - %s: %.1f GB / %.1f GB (%.1f%% used)\n", d.Mount, usedGB, totalGB, d.PercentUsed)
		}
	}
	return sb.String()
}
