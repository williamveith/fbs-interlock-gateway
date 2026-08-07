# Security Policy

## Supported Versions

Security updates are provided for the latest released version of FBS Interlock Gateway.

| Version        | Supported |
| -------------- | --------- |
| Latest release | Yes       |
| Older releases | No        |

Before reporting a vulnerability, confirm that it is reproducible against the latest release or the current default branch.

## Reporting a Vulnerability

Please report suspected security vulnerabilities through **GitHub Private Vulnerability Reporting** in the repository’s **Security** tab.

Do **not** report security vulnerabilities through:

* Public GitHub issues
* Public pull requests
* Discussions
* Social media
* Public mailing lists

A useful report should include:

* A description of the vulnerability and its potential impact
* The affected version, commit, operating system, and deployment type
* Reproduction steps or a minimal proof of concept
* Relevant configuration with passwords, private keys, certificates, hostnames, and other secrets removed
* Any conditions required to exploit the issue
* Suggested mitigations, when available

Reports should contain enough information to reproduce and evaluate the issue without requiring access to a production deployment.

## Response Process

The maintainer will make a reasonable effort to:

1. Acknowledge the report within three business days.
2. Perform an initial assessment within seven business days.
3. Confirm whether the issue is accepted, requires additional information, or is outside the scope of this policy.
4. Develop and test an appropriate correction based on the severity and complexity of the issue.
5. Coordinate disclosure after a corrected release or mitigation is available.

These timeframes are targets and may vary depending on the issue’s complexity, available information, and operational impact.

## Security Scope

Security issues may include, but are not limited to:

* Authentication or authorization bypasses
* Exposure of passwords, private keys, certificates, or configuration secrets
* Improper validation of client or server certificates
* mTLS implementation or certificate-handling weaknesses
* Unauthorized Shelly RPC commands or relay control
* Unauthorized access to the administrative interface
* Remote code execution
* Command, path, configuration, or environment-variable injection
* Privilege escalation
* Unsafe file permissions
* Installer, updater, or package-integrity vulnerabilities
* Insecure defaults that could expose the service or managed devices
* Denial-of-service conditions that materially affect gateway operation
* Vulnerabilities that could cause an interlock to enter an unintended state
* Circumvention of configured safe-state behavior
* Cross-tool access caused by incorrect routing, port handling, or configuration isolation

## Operational Safety

FBS Interlock Gateway controls physical interlock devices. Security testing must not create a risk to personnel, equipment, facilities, or ongoing operations.

Researchers must not:

* Test against a production deployment without explicit written authorization
* Energize, de-energize, or repeatedly cycle connected equipment
* Change the state of an active laboratory interlock
* Disable or circumvent facility safety controls
* Interrupt legitimate equipment use
* Modify or delete production configuration
* Access data belonging to other users or organizations
* Perform destructive testing
* Conduct denial-of-service testing against production systems
* Scan networks or devices outside the minimum scope necessary to validate the issue

Testing should be performed using an isolated development environment, test gateway, simulated device, or dedicated non-production Shelly device whenever possible.

Stop testing immediately if an action could affect physical equipment, safety systems, or production availability.

## Secrets and Certificates

Do not include active credentials or private material in a vulnerability report, issue, pull request, log, screenshot, or proof of concept.

This includes:

* Shelly usernames and passwords
* TLS private keys
* Client certificates
* Certificate-authority private keys
* Production configuration files
* Internal hostnames or network addresses
* GitHub tokens
* Signing keys
* Deployment credentials

If a report requires demonstrating secret exposure, redact the secret and provide only the minimum evidence necessary to establish that exposure occurred.

Any credential or private key believed to have been exposed should be treated as compromised and rotated.

## Out-of-Scope Reports

The following are generally outside the scope of this policy unless they create a direct, demonstrable vulnerability in FBS Interlock Gateway:

* Vulnerabilities that only affect unsupported versions
* Issues that cannot be reproduced
* Theoretical concerns without a practical security impact
* Missing security headers that do not produce an exploitable condition
* Self-XSS or attacks requiring a user to execute arbitrary code manually
* Social engineering, phishing, or physical access attacks
* Vulnerabilities in an unmodified third-party dependency with no demonstrated impact on this project
* Network outages, Wi-Fi instability, device firmware defects, or hardware failures
* Reports based solely on automated scanner output
* Brute-force, load, or denial-of-service testing performed without authorization
* Exposure caused by an operator intentionally publishing credentials or deploying the service with insecure permissions contrary to the documentation

Dependency vulnerabilities are still welcome when the report explains how the affected dependency is reachable and exploitable through this project.

## Coordinated Disclosure

Please allow the maintainer a reasonable opportunity to investigate and correct a vulnerability before publicly disclosing it.

Do not publish:

* Exploit code
* Detailed reproduction steps
* Screenshots containing sensitive information
* Active credentials or certificates
* Information identifying a vulnerable production deployment

The maintainer may publish a GitHub Security Advisory, corrected release, release notes, mitigation instructions, or CVE when appropriate.

Public credit may be provided to the reporter unless anonymity is requested.

## Safe Harbor

Security research conducted in good faith and in accordance with this policy will be considered authorized for the limited purpose of identifying and reporting vulnerabilities.

Researchers are expected to:

* Avoid privacy violations
* Avoid accessing more information than necessary
* Avoid persistence after demonstrating the issue
* Avoid disruption or damage
* Report findings promptly
* Follow coordinated-disclosure requirements
* Comply with applicable laws and organizational policies

This safe-harbor statement does not authorize testing against systems, networks, devices, or facilities that the researcher does not own or have explicit permission to test.
