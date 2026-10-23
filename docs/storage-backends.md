# Storage backends

A storage is given by a URL to `duplicacy init`, `add` or `info`. This page lists the URL forms, the credentials
each backend asks for, and backend-specific notes. Everything here is taken from `CreateStorage` in
`src/duplicacy_storage.go`.

## Credentials

Each credential is looked up, in order, in:

1. **Environment variables**: `DUPLICACY_<KEY>` for the default storage, `DUPLICACY_<NAME>_<KEY>` for a storage
   added under `<NAME>`, upper-case. If the storage name contains characters other than letters, digits and `_`,
   the same name with those characters replaced by `_` is also tried. For example `DUPLICACY_PASSWORD`,
   `DUPLICACY_S3_SECRET`, `DUPLICACY_OFFSITE_B2_KEY`.
2. **The `keys` section of `.duplicacy/preferences`** (set with `duplicacy set -key <key> -value <value>`). The
   file is created readable only by its owner, but the values are stored in plain text.
3. **The system keyring** (macOS Keychain, Secret Service on Linux).
4. **A prompt**. What is entered is saved to the keyring, unless the storage was configured with
   `duplicacy set -no-save-password`.

With `-background`, prompts are never shown: credentials must come from one of the first three sources.

| Key | Used for |
|-----|----------|
| `password` | The storage password (encrypted storages) |
| `rsa_passphrase` | The passphrase of the RSA private key given with `-key` |
| `ssh_password`, `ssh_key_file`, `ssh_passphrase` | SFTP login (see below) |
| `s3_id`, `s3_secret` | S3, S3-compatible and MinIO |
| `wasabi_key`, `wasabi_secret` | Wasabi |
| `b2_id`, `b2_key` | Backblaze B2 account or application key id, and the key |
| `azure_key` | Azure storage account access key |
| `dropbox_token` | Dropbox refresh token |
| `gcs_token` | Path of a Google Cloud Storage token or service-account file |
| `gcd_token` | Path of a Google Drive token file |
| `one_token`, `odb_token` (+ `_client_id`, `_client_secret`) | Path of a OneDrive (personal / business) token file, optional own app credentials |
| `acd_token`, `hubic_token` | Token file paths for Amazon Cloud Drive and Hubic (both services are discontinued) |
| `swift_key` | OpenStack Swift key |
| `webdav_password` | WebDAV password |
| `fabric_token` | Storage Made Easy File Fabric token |
| `storj_key`, `storj_passphrase` | Storj API key and encryption passphrase |
| `smb_password` | SMB password |

Prompts for secrets don't echo what you type; prompts for ids and file paths do.

## URL forms

| Backend | URL | Notes |
|---------|-----|-------|
| Local disk | `/path/to/storage` | Strongly consistent. Files are written to a temporary name and renamed. |
| SFTP | `sftp://user@host[:port]/path` | Host keys are checked against `known_hosts` files (see below). Strongly consistent. |
| SFTP (legacy algorithms) | `sftpc://user@host[:port]/path` | Also allows arcfour, 3des-cbc and diffie-hellman-group1-sha1 for old servers; prints a warning. |
| SMB | `smb://user@host[:port]/share/path` | Strongly consistent. |
| Amazon S3 | `s3://region@amazon.com/bucket/path` | `amazon` or `amazon.com` selects AWS; eventually consistent handling. |
| S3-compatible | `s3://region@endpoint/bucket/path` | Any S3 API endpoint over HTTPS. |
| S3 (goamz client) | `s3c://region@endpoint/bucket/path` | Alternative S3 client for endpoints the AWS SDK doesn't work with. |
| MinIO | `minio://region@host[:port]/bucket/path` (HTTP), `minios://…` (HTTPS) | Path-style requests. `minio://` is unencrypted on the network. |
| Wasabi | `wasabi://region@s3.wasabisys.com/bucket/path` | S3 with Wasabi-specific renames. Strongly consistent. |
| Backblaze B2 | `b2://bucket[/path]` | Strongly consistent. |
| B2 with a custom download host | `b2-custom://download-host/bucket[/path]` | Downloads go through the given host (for example a CDN). |
| Azure | `azure://account/container` | Strongly consistent. |
| Google Cloud Storage | `gcs://bucket[/path]` | Strongly consistent. |
| Google Drive | `gcd://path` or `gcd://drive@path` | `drive@` selects a shared drive by its name. |
| OneDrive | `one://path`, `odb://path` (business), or `odb://drive-id@path` | `drive-id@` selects a drive other than the user's own by its id. |
| Dropbox | `dropbox://path` | |
| OpenStack Swift | `swift://user@host[:port]/container[/path][?options]` | Options such as `protocol=http`, `timeout=<seconds>`, `connection_timeout=<seconds>`, `auth_version=<n>`. |
| WebDAV | `webdav://user@host[:port]/path` (HTTPS), `webdav-http://…` (HTTP) | `webdav-http://` is unencrypted on the network. |
| File Fabric | `fabric://endpoint/path` | |
| Storj | `storj://satellite/bucket[/path]` | |
| Amazon Cloud Drive, Hubic | `acd://path`, `hubic://path` | Services discontinued; kept for existing configurations. |

"Strongly consistent" storages get a 15-minute safety margin in prune instead of 12 hours; see
[PRUNE_SAFETY.md](PRUNE_SAFETY.md).

## SFTP details

- **Host keys** are checked against `.duplicacy/known_hosts`, `~/.ssh/known_hosts` and `/etc/ssh/ssh_known_hosts`,
  by host name. A new key is shown with its fingerprint and must be confirmed; in `-background` mode or without a
  terminal it is refused unless `DUPLICACY_SSH_ACCEPT_NEW_HOST_KEY=1` is set. A changed key is always refused.
- **Login**: keys from a running `ssh-agent` (`SSH_AUTH_SOCK`) and a configured key file (`ssh_key_file`, with
  `ssh_passphrase` if encrypted) are tried before a password. A certificate next to the key file
  (`<key file>-cert.pub`) is used automatically.
- The connection attempt times out after 30 seconds; transient errors during transfers are retried.

## Behaviour common to all backends

- Chunk uploads that fail are retried up to three times (after 2, 8 and 30 seconds) before the backup stops;
  downloads of existing chunks are retried the same way.
- The backends with their own HTTP client (B2, OneDrive, WebDAV, File Fabric, Wasabi renames) keep enough idle
  connections for every thread to reuse one, and give up on stalled connections (30 s to connect or complete the
  TLS handshake, 5 minutes for a response to start).
- Rate limits: `-limit-rate <kB/s>` on backup and restore, `-upload-limit-rate` and `-download-limit-rate` on copy.

## Separate metadata directory

A `nesting` file in the storage root can move metadata chunks to `metadata/` (so that, for example, `chunks/` can
use a cheaper, slower storage class):

```json
{"read-levels": [1], "write-level": 1, "separate-metadata": true}
```

Metadata chunks written before the switch are still found under `chunks/`. See [FORMAT.md](FORMAT.md).
