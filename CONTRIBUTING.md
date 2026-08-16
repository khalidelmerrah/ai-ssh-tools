# Contributing to ai-ssh-tools

Thank you for your interest in contributing to `ai-ssh-tools`! 

## 🛠️ Development Setup

### Prerequisites
- **Go 1.22+**
- **Git**

### Clone & Run Tests
```bash
git clone https://github.com/khalidelmerrah/ai-ssh-tools.git
cd ai-ssh-tools

# Run all test suites
go test -v ./...

# Build binary locally
go build -ldflags="-s -w" -o ai-ssh-tools ./cmd/ai-ssh-tools
```

---

## 📐 Project Structure

```
├── cmd/
│   └── ai-ssh-tools/      # Entry point, CLI router, MCP handlers
├── pkg/
│   ├── security/          # Command sanitizer, rate limiting, audit logging
│   ├── sshconfig/         # ~/.ssh/config parser & wildcard matcher
│   └── vitals/            # Remote system diagnostics & JSON formatting
├── .github/
│   └── workflows/         # Automated release CI/CD
└── build.ps1 / build.sh   # Cross-compilation scripts
```

---

## 📝 Pull Request Guidelines

1. **Keep it focused**: One bug fix or feature per pull request.
2. **Add tests**: Ensure new functionality includes unit tests in the appropriate package.
3. **Format code**: Run `gofmt -s -w .` before committing.
4. **Pass all checks**: Make sure `go test ./...` passes without errors.

---

## 💬 Community

Feel free to open an issue for bug reports, feature requests, or questions.
