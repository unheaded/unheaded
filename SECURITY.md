# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Below is a table indicating which versions currently receive security updates:

| Version | Supported          |
|---------|--------------------|
| 1.0.x   | Yes                |
| < 1.0   | No                 |

## Reporting a Vulnerability

We take the security of Unheaded seriously. If you believe you have found a security vulnerability, please report it to us as described below.

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

### How to Report

Email your findings to: **stevenrbellis@gmail.com**

Include the following information to help us better understand and resolve the issue:

- **Type of vulnerability** (e.g., buffer overflow, injection, authentication bypass, etc.)
- **Location** (file path, function name, line number if applicable)
- **Description** of the vulnerability and potential impact
- **Proof of concept** or reproducible steps
- **Affected versions** (which versions of Unheaded are impacted)
- **Your contact information** (optional, but recommended for follow-up)

## Disclosure Timeline

We aim to address all security vulnerabilities within our vulnerability disclosure timeline:

- **Immediate (within 24 hours)**: Acknowledge receipt of your vulnerability report
- **Up to 7 days**: Initial assessment and confirmation of the vulnerability
- **Up to 90 days**: Development and testing of the fix
- **90 days**: Public disclosure of the vulnerability and availability of the patch

During this time, we will:
1. Confirm the vulnerability and determine its scope
2. Develop and test a fix
3. Release a patch update with the fix
4. Publish a security advisory

If you have a preferred disclosure timeline (e.g., you represent a large enterprise or need more time), please mention it in your report.

## What to Include in Security Reports

A good security report should include:

- **Summary**: A brief description of the issue
- **Vulnerability Type**: Classification (OWASP Top 10, CWE, etc.)
- **Affected Component**: Which service, module, or feature is impacted
- **Severity Assessment**: Your assessment of the severity (Critical, High, Medium, Low)
- **Steps to Reproduce**: Detailed instructions to reproduce the vulnerability
- **Potential Impact**: What an attacker could do if this vulnerability is exploited
- **Mitigation**: Any workarounds or mitigation strategies you've identified

## What NOT to Report

Please do **not** report the following as security vulnerabilities:

- **Bugs in dependencies**: Report these to the dependency maintainers
- **Issues in customer applications**: These are outside the scope of Unheaded platform security
- **Social engineering or phishing**: Contact your local law enforcement
- **DoS attacks or performance issues** that don't involve a security vulnerability
- **Information disclosure from public sources** (e.g., published documentation, public repositories)
- **Features you wish existed** (submit as feature requests instead)

## Scope

### In Scope

The Unheaded platform infrastructure includes:

- **Core services**: Wotan (message bus), Timeguru (timeline), Captain (vision), Architect (ADRs), Micromanager (tasks)
- **State management**: Monad (unified state), Sophia (knowledge graph)
- **API gateway**: Gateway (public access point)
- **System components**: Dashboard backend, eBPF infrastructure, trace collection
- **Configuration management**: All configuration files and environment handling
- **Authentication and authorization**: Token handling, permission enforcement
- **Data protection**: Encryption, key management, secure deletion
- **Network security**: TLS/SSL configuration, network isolation, firewall rules
- **Access control**: Role-based access control (RBAC), audit logging

### Out of Scope

The following are **not** considered within the security scope of Unheaded:

- **Customer applications** built on top of Unheaded
- **Third-party integrations** that use Unheaded APIs
- **User data stored in customer databases** (responsibility of customer)
- **Infrastructure security** of customer deployments
- **Vulnerabilities in dependencies** (report to dependency maintainers)
- **Social engineering** targeting users of Unheaded
- **Physical security** of infrastructure

## General Security Practices

While using Unheaded, please follow these security best practices:

### Deployment

- Keep Unheaded and all dependencies updated to the latest version
- Run services with minimal required permissions
- Use TLS/SSL for all network communication
- Implement network segmentation and firewall rules
- Enable audit logging and monitor for suspicious activity
- Use strong authentication mechanisms (mTLS, API keys, tokens)

### Configuration

- Never commit secrets to version control
- Use environment variables or secure vaults for sensitive configuration
- Review and harden default security settings
- Implement rate limiting on public endpoints
- Enable CORS only for trusted origins
- Implement request body size limits on HTTP servers

### Monitoring

- Monitor system logs for security events
- Set up alerts for failed authentication attempts
- Monitor resource usage for signs of attacks
- Implement distributed tracing to detect anomalies
- Use eBPF monitoring for deep system visibility

## Security Headers

Unheaded services implement the following security headers:

- `Strict-Transport-Security (HSTS)`: Enforce HTTPS
- `X-Content-Type-Options: nosniff`: Prevent MIME type sniffing
- `X-Frame-Options: DENY`: Prevent clickjacking
- `Content-Security-Policy`: Restrict resource loading
- `X-XSS-Protection`: Enable XSS protection

## Cryptography

Unheaded uses the following cryptographic standards:

- **TLS**: Version 1.2 or higher
- **Cipher Suites**: Modern, secure algorithms (no deprecated ciphers)
- **Key Exchange**: ECDHE for perfect forward secrecy
- **Authentication**: X.509 certificates with proper validation
- **Hash Functions**: SHA-256 or stronger
- **Symmetric Encryption**: AES-256-GCM

## Acknowledgments

We appreciate the security research community and acknowledge researchers who report vulnerabilities responsibly. Upon request and with your permission, we will credit you in our security advisories.

## Contact

For security-related questions or concerns:

- **Security vulnerability reports**: stevenrbellis@gmail.com
- **General inquiries**: See repository README for contact information

---

**Last updated**: 2026-02-20
**Version**: 1.0
