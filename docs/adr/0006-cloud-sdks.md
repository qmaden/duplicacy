# 0006: Current cloud SDKs, chosen for size, and fakes to test them

- Status: Accepted (2026-10-23)

## Context

The S3, Azure and Google backends used SDKs from 2018–2020: aws-sdk-go v1 (end of support), a fork of the Azure
SDK pulling in the unmaintained `dgrijalva/jwt-go`, and Google API clients pinned with gRPC 1.28, which carried
the remaining govulncheck findings. Upgrades had been deferred because none of these backends could be tested
without accounts on the services.

The current Google Cloud Storage client (`cloud.google.com/go/storage`) also brings gRPC transport,
OpenTelemetry and Cloud Monitoring, growing the binary from 28 MB to 47 MB, while the backend makes six plain
HTTP calls (list, get, insert, delete, rewrite, metadata).

## Decision

- S3, MinIO and Wasabi use `aws-sdk-go-v2`, with request checksums only where the API requires them (the v2
  default adds CRC checksums that many S3-compatible services reject). `s3c://` keeps the goamz client as a
  fallback.
- Azure uses the official `azblob` SDK with one client shared by all threads.
- Google Cloud Storage uses the generated Cloud Storage JSON API client from `google.golang.org/api`, the module
  Google Drive already uses. The binary grows by 2.6 MB instead of 19 MB.
- Every backend gets a fake or an emulator that the storage, backup, prune and copy tests run against in CI:
  `gofakes3` for S3, `fake-gcs-server` for GCS, a Drive API fake written for the operations the backend uses,
  and Azurite for Azure. Each upgrade was also checked against an independent real server where one was
  available (versitygw for S3) and, for S3 and msgpack, by restoring across the old and new builds.

## Consequences

- Running these tests found three bugs in code that had never been exercised without real accounts: failed GCS
  uploads reported as successes, a hang when listing chunks on Google Drive, and a crash with per-thread
  backends after parallel metadata downloads were added.
- Fakes don't reproduce the services' quirks (eventual consistency, rate limits, authentication). The backends
  still need a run against the real services before a release.
- The Drive fake implements only the queries the backend sends; a change to those queries must update the fake.
- Revisit the GCS client if a feature needs the official library (for example, gRPC transport for throughput).
