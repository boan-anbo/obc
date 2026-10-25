# Explicit CLI updates

`ob update --source https://github.com/boan-anbo/obc/releases/latest/download/cli-manifest.json`
loads `openbibliographies.cli-update.v1`, separate from the data manifest.
The manifest binds the exact compressed bundle, original catalog, author mapping,
reader/source version and every CLI release asset. GitHub URLs are immutable
versioned URLs inside the manifest; the latest URL selects a release.

The update downloads data only, never executes a downloaded binary, validates
all bindings and atomically changes the cache pointer. Queries never update
implicitly. Failed updates preserve the previous cache. A local bundle can be
loaded with `ob update --bundle FILE`. Snapshots must remain fixed during paging.

`CLI-SHA256SUMS` covers CLI payloads. `SHA256SUMS` covers the separate data assets.
Checksums provide integrity under a trusted manifest; they are not signatures.
The publisher uploads and verifies immutable assets before selecting latest.
Code is MIT (CLI-LICENSE), compiled data/classifications CC BY4.0 (DATA-LICENSE).
