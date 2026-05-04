# Security Policy

Agent VM manages local agent configuration and may eventually render files used
by AI coding tools. Treat security reports seriously, especially when they
involve secrets, unsafe file writes, permission bypasses, or untrusted profile
imports.

## Supported Versions

AVM has not published a stable release yet. Security fixes currently target the
main branch.

## Reporting a Vulnerability

If GitHub private vulnerability reporting is enabled for the repository, use it.
If it is not enabled, contact the maintainer through GitHub and avoid posting
exploit details publicly until there is a coordinated fix path.

Please include:

- affected command or package
- reproduction steps
- expected impact
- whether secrets or arbitrary file writes are involved
- any relevant logs with tokens and credentials removed

## Security Principles

- Do not export plaintext credentials.
- Prefer environment variable references for secrets.
- Keep runtime writes limited to adapter-owned managed paths.
- Preserve backups before overwriting runtime-managed files.
- Do not add runtime-native memory import/export without an explicit design,
  audit trail, and user confirmation model.
- Report unsupported runtime mappings instead of silently dropping them.
