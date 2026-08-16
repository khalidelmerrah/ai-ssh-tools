# Security Policy

## 🛡️ Supported Versions

We release patches and security fixes for the latest version of `ai-ssh-tools`.

| Version | Supported          |
| ------- | ------------------ |
| 1.1.x   | :white_check_mark: |
| < 1.1.0 | :x:                |

---

## 🔒 Security Posture & Guarantees

1. **Local Credential Isolation**: Private keys and passwords are processed exclusively in local memory and are never transmitted over MCP protocols, written to logs, or exposed in AI context prompts.
2. **Command Injection Prevention**: Input commands are sanitized against unsafe chaining tokens (`;`, `&&`, `||`, backticks, `$()`) and newlines (`\r\n`).
3. **Host Verification**: Trust-On-First-Use (TOFU) SHA256 fingerprint verification protects against active Man-in-the-Middle (MITM) attacks.
4. **Audit Logging**: All executed actions are recorded to a local append-only JSON audit log (`~/.ai-ssh-tools/audit.log`) with zero secret disclosure.

---

## 🚨 Reporting a Vulnerability

If you discover a security vulnerability within `ai-ssh-tools`, please report it responsibly:

- **Do NOT open a public issue.**
- Submit a private vulnerability advisory on GitHub via the **Security** tab of the repository.
- Include detailed steps to reproduce the vulnerability.

We will review reports promptly and publish patches in a timely release.
