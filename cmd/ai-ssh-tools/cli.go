package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// runCLI parses and routes CLI subcommands.
func runCLI(args []string) error {
	if len(args) == 0 {
		return nil
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "serve", "--mcp":
		return nil // continue to MCP server
	case "exec":
		return runExecCLI(subArgs)
	case "vitals":
		return runVitalsCLI(subArgs)
	case "transfer":
		return runTransferCLI(subArgs)
	case "docker":
		return runDockerCLI(subArgs)
	case "service":
		return runServiceCLI(subArgs)
	case "tail":
		return runTailCLI(subArgs)
	case "profiles":
		return runProfilesCLI(subArgs)
	case "help", "--help", "-h":
		printHelp()
		os.Exit(0)
		return nil
	case "version", "--version", "-v":
		fmt.Println("ai-ssh-tools v1.1.0")
		os.Exit(0)
		return nil
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'ai-ssh-tools --help' for usage.\n", cmd)
		os.Exit(1)
		return nil
	}
}

func printHelp() {
	fmt.Println(`ai-ssh-tools — SSH Operations Bridge for AI Workbenches & Terminal

USAGE:
  ai-ssh-tools [command] [options]

COMMANDS:
  serve                      Run as Model Context Protocol (MCP) server on stdio (default)
  exec [options] <command>   Execute a remote command over SSH
  vitals [options]           Fetch system metrics (RAM, CPU, Disk, OS, Uptime)
  transfer [options]         Stream files via SFTP (upload/download)
  docker [options]           List and inspect Docker containers
  service [options]          Manage systemd services (status, restart, logs, etc.)
  tail [options]             Tail remote log files
  profiles [list|add]        Manage local SSH profiles

OPTIONS (Common across commands):
  --host <ip|domain>         Remote host IP or domain name (or ~/.ssh/config alias)
  --user <username>          SSH username
  --port <port>              SSH port (default: 22)
  --key <path>               Path to private SSH key
  --profile <alias>          Named profile from ssh_hosts.json or ~/.ssh/config
  --timeout <seconds>        Command timeout in seconds (default: 30)

EXEC OPTIONS:
  --sudo                     Run command with sudo privileges
  --pty                      Allocate pseudo-terminal (PTY)
  --workdir <path>           Remote working directory
  --git                      Wrap command with pre/post Git rollback snapshots

TRANSFER OPTIONS:
  --src <path>               Source path (local for upload, remote for download)
  --dst <path>               Destination path (remote for upload, local for download)
  --op <upload|download>     Transfer operation (default: upload)

SERVICE OPTIONS:
  --name <service>           Service unit name (e.g. nginx, docker)
  --action <action>          status | start | stop | restart | reload | enable | disable | logs
  --lines <num>              Log lines for 'logs' action (default: 50)
  --sudo                     Run systemctl command with sudo

DOCKER OPTIONS:
  --all                      Include stopped containers (docker ps -a)

TAIL OPTIONS:
  --path <path>              Remote file path to tail
  --lines <num>              Number of lines to read (default: 50)

EXAMPLES:
  # Execute remote command via direct host/user
  ai-ssh-tools exec --host 192.168.1.50 --user deploy "uptime"

  # Run with sudo and pty
  ai-ssh-tools exec --host 192.168.1.50 --user deploy --sudo "systemctl restart nginx"

  # Check system vitals
  ai-ssh-tools vitals --host 192.168.1.50 --user deploy

  # Transfer a file
  ai-ssh-tools transfer --host 192.168.1.50 --user deploy --src ./build.tar.gz --dst /opt/app/build.tar.gz

  # Inspect Docker containers
  ai-ssh-tools docker --host 192.168.1.50 --user deploy

  # Run as MCP Server for AI Assistants
  ai-ssh-tools serve`)
}

func runExecCLI(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile alias")
	host := fs.String("host", "", "Remote host")
	user := fs.String("user", "", "Remote user")
	port := fs.Int("port", 22, "Remote port")
	keyPath := fs.String("key", "", "Key path")
	workdir := fs.String("workdir", "", "Working directory")
	gitWrapped := fs.Bool("git", false, "Wrap in git snapshots")
	sudo := fs.Bool("sudo", false, "Execute with sudo")
	pty := fs.Bool("pty", false, "Allocate pseudo-terminal")
	timeoutSec := fs.Int("timeout", 30, "Timeout in seconds")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Error: command string is required. Example: ai-ssh-tools exec --host 1.2.3.4 --user root 'uptime'")
		os.Exit(1)
	}
	cmdStr := strings.Join(remaining, " ")

	prof, err := resolveProfile(*profile, *host, *user, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving connection: %v\n", err)
		os.Exit(1)
	}
	if *keyPath != "" {
		prof.KeyPath = *keyPath
	}

	cleanCmd, err := validateCommandPolicy(prof, cmdStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Security policy error: %v\n", err)
		os.Exit(1)
	}

	client, err := getOrConnect(prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH connection error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
	defer cancel()

	var res *execResult
	if *gitWrapped && *workdir != "" {
		res, err = gitWrappedExec(ctx, client, *workdir, cleanCmd, *pty, *sudo, "")
	} else {
		execCmd := cleanCmd
		if *workdir != "" {
			execCmd = fmt.Sprintf("cd %s && %s", shellQuote(*workdir), cleanCmd)
		}
		res, err = remoteExecOpts(ctx, client, execCmd, *pty, *sudo, "")
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution failed: %v\n", err)
		os.Exit(1)
	}

	if res.Stdout != "" {
		fmt.Print(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			fmt.Println()
		}
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			fmt.Fprintln(os.Stderr)
		}
	}
	os.Exit(res.ExitCode)
	return nil
}

func runVitalsCLI(args []string) error {
	fs := flag.NewFlagSet("vitals", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile alias")
	host := fs.String("host", "", "Remote host")
	user := fs.String("user", "", "Remote user")
	port := fs.Int("port", 22, "Remote port")
	keyPath := fs.String("key", "", "Key path")
	jsonOut := fs.Bool("json", false, "Output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	prof, err := resolveProfile(*profile, *host, *user, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if *keyPath != "" {
		prof.KeyPath = *keyPath
	}

	client, err := getOrConnect(prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vitals, err := fetchVitals(ctx, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to collect vitals: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		data, _ := json.MarshalIndent(vitals, "", "  ")
		fmt.Println(string(data))
		os.Exit(0)
	}

	fmt.Printf("--- System Vitals: %s (%s) ---\n", prof.Alias, vitals.OSName)
	fmt.Printf("Uptime:       %d seconds\n", vitals.UptimeSeconds)
	if len(vitals.LoadAverages) >= 3 {
		fmt.Printf("Load Avg:     1m: %.2f | 5m: %.2f | 15m: %.2f\n", vitals.LoadAverages[0], vitals.LoadAverages[1], vitals.LoadAverages[2])
	}
	usedMB := vitals.MemoryBytes.Used / (1024 * 1024)
	totalMB := vitals.MemoryBytes.Total / (1024 * 1024)
	fmt.Printf("Memory:       %d MB / %d MB (%.1f%% used)\n", usedMB, totalMB, vitals.MemoryBytes.PercentUsed)

	if len(vitals.Disks) > 0 {
		fmt.Println("Disk Mounts:")
		for _, d := range vitals.Disks {
			usedGB := float64(d.UsedBytes) / (1024 * 1024 * 1024)
			totalGB := float64(d.TotalBytes) / (1024 * 1024 * 1024)
			fmt.Printf("  - %s: %.1f GB / %.1f GB (%.1f%% used)\n", d.Mount, usedGB, totalGB, d.PercentUsed)
		}
	}
	os.Exit(0)
	return nil
}

func runTransferCLI(args []string) error {
	fs := flag.NewFlagSet("transfer", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile alias")
	host := fs.String("host", "", "Remote host")
	user := fs.String("user", "", "Remote user")
	port := fs.Int("port", 22, "Remote port")
	keyPath := fs.String("key", "", "Key path")
	src := fs.String("src", "", "Source path")
	dst := fs.String("dst", "", "Destination path")
	direction := fs.String("op", "upload", "Transfer direction (upload/download)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *src == "" || *dst == "" {
		fmt.Fprintln(os.Stderr, "Error: both --src and --dst are required")
		os.Exit(1)
	}

	prof, err := resolveProfile(*profile, *host, *user, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if *keyPath != "" {
		prof.KeyPath = *keyPath
	}

	client, err := getOrConnect(prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	n, err := transferFileStream(client, *src, *dst, *direction)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Transfer failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Successfully transferred %d bytes (%s -> %s)\n", n, *src, *dst)
	os.Exit(0)
	return nil
}

func runDockerCLI(args []string) error {
	fs := flag.NewFlagSet("docker", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile alias")
	host := fs.String("host", "", "Remote host")
	user := fs.String("user", "", "Remote user")
	port := fs.Int("port", 22, "Remote port")
	all := fs.Bool("all", false, "Show all containers including stopped")

	if err := fs.Parse(args); err != nil {
		return err
	}

	prof, err := resolveProfile(*profile, *host, *user, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client, err := getOrConnect(prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	cmd := `docker ps --format "{{json .}}"`
	if *all {
		cmd = `docker ps -a --format "{{json .}}"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := remoteExec(ctx, client, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution failed: %v\n", err)
		os.Exit(1)
	}
	if res.ExitCode != 0 {
		fmt.Fprintf(os.Stderr, "Docker error (exit %d): %s\n", res.ExitCode, res.Stderr)
		os.Exit(res.ExitCode)
	}

	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		fmt.Println("No running Docker containers found.")
		os.Exit(0)
	}

	fmt.Printf("%-14s %-25s %-20s %s\n", "CONTAINER ID", "IMAGE", "STATUS", "NAMES")
	fmt.Println(strings.Repeat("-", 80))
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err == nil {
			id, _ := m["ID"].(string)
			img, _ := m["Image"].(string)
			status, _ := m["Status"].(string)
			names, _ := m["Names"].(string)
			if len(id) > 12 {
				id = id[:12]
			}
			if len(img) > 24 {
				img = img[:21] + "..."
			}
			fmt.Printf("%-14s %-25s %-20s %s\n", id, img, status, names)
		}
	}
	os.Exit(0)
	return nil
}

func runServiceCLI(args []string) error {
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile alias")
	host := fs.String("host", "", "Remote host")
	user := fs.String("user", "", "Remote user")
	port := fs.Int("port", 22, "Remote port")
	name := fs.String("name", "", "Service name (e.g. nginx, docker)")
	action := fs.String("action", "status", "Action (status, start, stop, restart, logs)")
	lines := fs.Int("lines", 50, "Log lines")
	sudo := fs.Bool("sudo", false, "Use sudo")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name <service> is required")
		os.Exit(1)
	}

	prof, err := resolveProfile(*profile, *host, *user, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client, err := getOrConnect(prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	var cmd string
	if *action == "logs" {
		cmd = fmt.Sprintf("journalctl -u %s -n %d --no-pager", shellQuote(*name), *lines)
	} else {
		cmd = fmt.Sprintf("systemctl %s %s", *action, shellQuote(*name))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := remoteExecOpts(ctx, client, cmd, false, *sudo, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Command failed: %v\n", err)
		os.Exit(1)
	}

	if res.Stdout != "" {
		fmt.Print(res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
	}
	os.Exit(res.ExitCode)
	return nil
}

func runTailCLI(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ExitOnError)
	profile := fs.String("profile", "", "Profile alias")
	host := fs.String("host", "", "Remote host")
	user := fs.String("user", "", "Remote user")
	port := fs.Int("port", 22, "Remote port")
	path := fs.String("path", "", "Remote file path")
	lines := fs.Int("lines", 50, "Number of lines")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *path == "" {
		fmt.Fprintln(os.Stderr, "Error: --path is required")
		os.Exit(1)
	}

	prof, err := resolveProfile(*profile, *host, *user, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client, err := getOrConnect(prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := fmt.Sprintf("tail -n %d %s", *lines, shellQuote(*path))
	res, err := remoteExec(ctx, client, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Tail command failed: %v\n", err)
		os.Exit(1)
	}

	if res.Stdout != "" {
		fmt.Print(res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
	}
	os.Exit(res.ExitCode)
	return nil
}

func runProfilesCLI(args []string) error {
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}

	switch action {
	case "list":
		profileRegistryMu.RLock()
		defer profileRegistryMu.RUnlock()

		if len(profileRegistry) == 0 {
			fmt.Println("No profiles loaded in ssh_hosts.json.")
			os.Exit(0)
		}
		fmt.Printf("%-20s %-25s %-6s %-15s %s\n", "ALIAS", "HOST", "PORT", "USER", "READONLY")
		fmt.Println(strings.Repeat("-", 75))
		for _, p := range profileRegistry {
			fmt.Printf("%-20s %-25s %-6d %-15s %v\n", p.Alias, p.Host, p.Port, p.User, p.ReadOnly)
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Usage: ai-ssh-tools profiles list\n")
		os.Exit(1)
	}
	return nil
}
