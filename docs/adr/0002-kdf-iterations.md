# 0002: Raise PBKDF2 iterations for new storages instead of changing the KDF

- Status: Accepted (2026-10-22)

## Context

Storage keys are protected by PBKDF2-SHA256 over the storage password with 16,384 iterations, far below current
guidance (600,000 or more, or a memory-hard function such as Argon2id). Anyone who can read the `config` file can
guess passwords offline at that cost.

The iteration count is already stored in each config file, so older clients honour any count. A new KDF would
need a new config format, which older clients (and the Duplicacy GUI) can't read.

## Decision

- New storages use 600,000 iterations (`CONFIG_DEFAULT_ITERATIONS`).
- `duplicacy password -iterations N` re-wraps an existing storage's keys, optionally with the same password.
- Counts above 100,000,000 in a config are rejected, so a planted config can't make the client spin.
- Warnings for passwords under 12 characters and for fewer than 100,000 iterations.
- No Argon2id for now.

## Consequences

- Every client version can still open every storage. Deriving the keys takes longer (a fraction of a second on a
  current machine, more on small devices), once per command.
- Existing storages stay weak until their owner runs `password -iterations`; UPGRADING.md says so.
- Revisit Argon2id together with any other change that needs a new config version, so that compatibility is
  broken at most once.
