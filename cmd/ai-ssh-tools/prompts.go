package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// promptArg safely fetches a string argument from the prompt request map.
func promptArg(req *mcp.GetPromptRequest, key string) string {
	if req.Params.Arguments == nil {
		return ""
	}
	return req.Params.Arguments[key]
}

// promptArgInt safely fetches an integer argument from the prompt request map.
func promptArgInt(req *mcp.GetPromptRequest, key string, defaultVal int) int {
	s := promptArg(req, key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

// promptArgBool safely fetches a boolean argument from the prompt request map.
func promptArgBool(req *mcp.GetPromptRequest, key string) bool {
	s := strings.ToLower(promptArg(req, key))
	return s == "true" || s == "1" || s == "yes"
}

func handleDiagnosePrompt(
	ctx context.Context,
	req *mcp.GetPromptRequest,
) (*mcp.GetPromptResult, error) {
	target := promptArg(req, "profile")
	if target == "" {
		target = promptArg(req, "host")
	}
	if target == "" {
		target = "<target>"
	}

	return &mcp.GetPromptResult{
		Description: "Sequential server vitals check",
		Messages: []*mcp.PromptMessage{
			{
				Role: mcp.Role("user"),
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(`Please diagnose the server "%s" by executing each of the following commands in sequence using the connect_and_execute tool. After each command, briefly summarise any anomalies before proceeding to the next.

Commands to run (one at a time):
1. df -h                        # Disk usage across all mount points
2. free -m                      # RAM and swap utilisation
3. ss -tulpn                    # Open TCP/UDP listening ports and associated processes
4. ps aux --sort=-%%mem | head -20  # Top 20 processes by memory consumption

After all four commands, provide a consolidated health summary highlighting any critical findings (disk > 85%%, swap in heavy use, unexpected open ports, or runaway processes).`, target),
				},
			},
		},
	}, nil
}

func handleLogsPrompt(
	ctx context.Context,
	req *mcp.GetPromptRequest,
) (*mcp.GetPromptResult, error) {
	target := promptArg(req, "profile")
	if target == "" {
		target = promptArg(req, "host")
	}
	lines := promptArgInt(req, "lines", 100)

	service := promptArg(req, "service")

	return &mcp.GetPromptResult{
		Description: "Fetch and analyse service logs",
		Messages: []*mcp.PromptMessage{
			{
				Role: mcp.Role("user"),
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(`Fetch the last %d lines of logs for service "%s" on "%s".

Try these commands in order (stop at the first that succeeds):
1. journalctl -u %s -n %d --no-pager
2. docker logs %s --tail %d 2>&1

Once you have the log output, analyse it for:
- Any ERROR, FATAL, CRITICAL, panic, or OOM (Out Of Memory) entries.
- Crash loops (repeated restarts or exit codes != 0).
- Abnormal latency spikes or timeout patterns.

Provide a structured summary: (a) what you found, (b) suspected root cause, (c) recommended next action.`,
						lines, service, target,
						service, lines,
						service, lines,
					),
				},
			},
		},
	}, nil
}

func handleDeployPrompt(
	ctx context.Context,
	req *mcp.GetPromptRequest,
) (*mcp.GetPromptResult, error) {
	target := promptArg(req, "profile")
	if target == "" {
		target = promptArg(req, "host")
	}
	branch := promptArg(req, "branch")
	if branch == "" {
		branch = "main"
	}
	workdir := promptArg(req, "workdir")
	service := promptArg(req, "service")
	pmName := promptArg(req, "pm2_name")
	migrate := promptArgBool(req, "migrate")

	var steps strings.Builder
	steps.WriteString(fmt.Sprintf(`Execute the following atomic deployment sequence on "%s" in the directory "%s". Each step must complete with exit code 0 before proceeding. If any step fails, STOP immediately and report the failure — do not attempt to skip or work around it.

Deployment Steps:
1. git fetch --all
2. git checkout %s
3. git pull origin %s`, target, workdir, branch, branch))

	// Detect package manager and add install step.
	steps.WriteString(`
4. Install dependencies:
   - If package.json exists: npm ci --no-fund --no-audit
   - If requirements.txt exists: pip install -r requirements.txt -q
   - If composer.json exists: composer install --no-interaction --no-dev --optimize-autoloader
   - If Gemfile exists: bundle install --without development test`)

	if migrate {
		steps.WriteString(`
5. Run migrations:
   - If artisan exists: php artisan migrate --force
   - If manage.py exists: python manage.py migrate --noinput
   - Otherwise: look for a Makefile target named 'migrate' or 'db:migrate'`)
	}

	if service != "" {
		steps.WriteString(fmt.Sprintf(`
%d. Restart service: sudo systemctl restart %s && sudo systemctl status %s`,
			map[bool]int{true: 6, false: 5}[migrate],
			service, service))
	} else if pmName != "" {
		steps.WriteString(fmt.Sprintf(`
%d. Restart PM2 process: pm2 restart %s --update-env`,
			map[bool]int{true: 6, false: 5}[migrate],
			pmName))
	}

	steps.WriteString(`

After the final step, confirm: (a) the deployed git commit hash, (b) service running status, (c) any warnings encountered during the process.`)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Atomic deployment of branch %s", branch),
		Messages: []*mcp.PromptMessage{
			{
				Role:    mcp.Role("user"),
				Content: &mcp.TextContent{Text: steps.String()},
			},
		},
	}, nil
}
