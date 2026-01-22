# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in MCP Proxy, please report it responsibly.

### How to Report

1. **Do NOT** open a public GitHub issue for security vulnerabilities
2. Email your findings to: security@agentfacts.dev (or open a private security advisory on GitHub)
3. Include as much detail as possible:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt within 48 hours
- **Assessment**: We will assess the vulnerability and determine its severity
- **Updates**: We will keep you informed of our progress
- **Resolution**: We aim to resolve critical issues within 7 days
- **Credit**: With your permission, we will credit you in the release notes

### Scope

The following are in scope for security reports:

- Authentication/authorization bypasses
- Policy enforcement failures
- Information disclosure
- Injection vulnerabilities
- Denial of service vulnerabilities
- Cryptographic issues

### Out of Scope

- Issues in dependencies (report to upstream maintainers)
- Issues requiring physical access
- Social engineering attacks
- Issues in third-party integrations

## Security Best Practices

When deploying MCP Proxy:

1. **Enable TLS** in production environments
2. **Use strong policies** - start with deny-all and allow explicitly
3. **Enable audit logging** for compliance and forensics
4. **Keep updated** - regularly update to the latest version
5. **Limit network exposure** - use firewalls and network segmentation
6. **Review policies** - regularly audit your OPA policies

## Security Features

MCP Proxy includes several security features:

- **OPA Policy Engine**: Fine-grained access control
- **Audit Logging**: Complete audit trail of all operations
- **TLS Support**: Encrypted communications
- **Session Management**: Secure session handling
- **Input Validation**: Request validation and sanitization

Thank you for helping keep MCP Proxy secure!
