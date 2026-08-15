package main

import (
	"context"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

type SystemVitals struct {
	OSName        string       `json:"os_name"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	LoadAverages  []float64    `json:"load_averages"`
	MemoryBytes   MemoryVitals `json:"memory_bytes"`
	Disks         []DiskVitals `json:"disks"`
}

type MemoryVitals struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	PercentUsed float64 `json:"percent_used"`
}

type DiskVitals struct {
	Mount       string  `json:"mount"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	PercentUsed float64 `json:"percent_used"`
}

func fetchVitals(ctx context.Context, client *ssh.Client) (*SystemVitals, error) {
	osRes, _ := remoteExec(ctx, client, "cat /etc/os-release")
	osName := "Linux (Unknown)"
	if osRes != nil && osRes.ExitCode == 0 {
		osName = parseOSName(osRes.Stdout)
	}

	uptimeRes, _ := remoteExec(ctx, client, "cat /proc/uptime")
	var uptime int64
	if uptimeRes != nil && uptimeRes.ExitCode == 0 {
		uptime = parseUptime(uptimeRes.Stdout)
	}

	loadRes, _ := remoteExec(ctx, client, "cat /proc/loadavg")
	var loads []float64
	if loadRes != nil && loadRes.ExitCode == 0 {
		loads = parseLoadAverages(loadRes.Stdout)
	}

	memRes, _ := remoteExec(ctx, client, "free -b")
	var mem MemoryVitals
	if memRes != nil && memRes.ExitCode == 0 {
		mem = parseMemoryBytes(memRes.Stdout)
	}

	diskRes, _ := remoteExec(ctx, client, "df -B1")
	var disks []DiskVitals
	if diskRes != nil && diskRes.ExitCode == 0 {
		disks = parseDisks(diskRes.Stdout)
	}

	return &SystemVitals{
		OSName:        osName,
		UptimeSeconds: uptime,
		LoadAverages:  loads,
		MemoryBytes:   mem,
		Disks:         disks,
	}, nil
}

func parseMemoryBytes(freeOutput string) MemoryVitals {
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

func parseDisks(dfOutput string) []DiskVitals {
	lines := strings.Split(dfOutput, "\n")
	var disks []DiskVitals
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 6 && !strings.HasPrefix(fields[0], "Filesystem") {
			total, _ := strconv.ParseUint(fields[1], 10, 64)
			used, _ := strconv.ParseUint(fields[2], 10, 64)
			free, _ := strconv.ParseUint(fields[3], 10, 64)
			mount := fields[5]
			
			// Only include real physical mount points (not devtmpfs, tmpfs, etc.)
			fs := fields[0]
			if strings.HasPrefix(fs, "/dev/") || mount == "/" {
				var pct float64
				if total > 0 {
					pct = (float64(used) / float64(total)) * 100.0
				}
				disks = append(disks, DiskVitals{
					Mount:       mount,
					TotalBytes:  total,
					UsedBytes:   used,
					FreeBytes:   free,
					PercentUsed: pct,
				})
			}
		}
	}
	return disks
}

func parseLoadAverages(loadavgOutput string) []float64 {
	fields := strings.Fields(loadavgOutput)
	var averages []float64
	if len(fields) >= 3 {
		for i := 0; i < 3; i++ {
			val, err := strconv.ParseFloat(fields[i], 64)
			if err == nil {
				averages = append(averages, val)
			}
		}
	}
	return averages
}

func parseUptime(uptimeOutput string) int64 {
	fields := strings.Fields(uptimeOutput)
	if len(fields) > 0 {
		parts := strings.Split(fields[0], ".")
		val, err := strconv.ParseInt(parts[0], 10, 64)
		if err == nil {
			return val
		}
	}
	return 0
}

func parseOSName(osReleaseOutput string) string {
	lines := strings.Split(osReleaseOutput, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(parts[1], `"` + `'`)
			}
		}
	}
	return "Linux (Generic)"
}
