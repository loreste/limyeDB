# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.2.x   | Yes                |
| 0.1.x   | No                 |

## Reporting a Vulnerability

We take the security of LimyeDB seriously. If you discover a security vulnerability, please report it responsibly.

### How to Report

- **GitHub Security Advisories** (preferred): [Report a vulnerability](https://github.com/loreste/limyeDB/security/advisories/new)
- **Email**: security@limyedb.io

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact
- Suggested fix (if any)

### What to Expect

- **Acknowledgment**: Within 48 hours of your report
- **Status update**: Within 5 business days
- **Fix timeline**: Critical vulnerabilities will be patched within 7 days; others within 30 days

### Scope

The following are in scope:

- Authentication and authorization bypasses
- SQL injection, command injection, path traversal
- Remote code execution
- Denial of service (resource exhaustion, crash bugs)
- Data exposure or leakage
- Cryptographic weaknesses

### Out of Scope

- Social engineering attacks
- Denial of service via network flooding
- Vulnerabilities in dependencies (report these upstream)
- Issues in development/test code only

### Recognition

We gratefully acknowledge security researchers who report vulnerabilities responsibly. With your permission, we will credit you in our release notes and security advisories.

## Security Hardening

LimyeDB includes multiple layers of security hardening. See the [Security section](README.md#security) in the README for details on constant-time token comparison, SSRF protection, path traversal prevention, and other measures.
