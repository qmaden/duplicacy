# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub security advisories](https://github.com/qmaden/duplicacy/security/advisories/new), not in public issues.
Include the version or commit, the platform, the storage backend, and steps to reproduce. Never include real
passwords, keys or backup data.

You can expect an acknowledgement within a few days. Fixes are released as soon as they are ready, and the
advisory is published once a fixed version is available.

## Supported versions

Only the latest release and the `master` branch of this fork receive security fixes. Supported platforms are
Linux x86-64 and macOS.

## Scope

[docs/SECURITY_MODEL.md](docs/SECURITY_MODEL.md) describes what Duplicacy protects against and what it doesn't.
Issues outside that model (for example, an attacker who already knows the storage password) are still welcome as
hardening suggestions, through normal issues.
