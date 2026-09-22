# Security Policy

## Supported versions

| Version | Supported |
| ------- | --------- |
| latest release | ✅ |
| older releases | ❌ |

## Reporting a vulnerability

wsh handles SSH credentials, so reports are taken seriously. If you discover a
security vulnerability, please **do not open a public issue**.

Instead, use GitHub's
[private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
on this repository, or contact the maintainer directly.

Please include:

- A description of the vulnerability and its impact
- Steps to reproduce or a proof of concept
- Affected versions/platforms

You can expect an initial response within 7 days. Please allow up to 90 days
for a fix before public disclosure.

## Security design notes

- The local HTTP/WebSocket service binds to `127.0.0.1` only.
- Passwords and private keys are encrypted with AES-GCM using a machine-local
  key (`secret.key`) before being written to disk.
- Passphrases are kept in memory only unless the user explicitly opts in.
