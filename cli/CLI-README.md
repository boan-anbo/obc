# ob — Open Bibliographies offline reader

Download the binary for macOS, Linux or Windows (amd64/arm64) from
https://github.com/boan-anbo/obc/releases and place it on PATH as `ob` (`ob.exe`).
Verify `CLI-SHA256SUMS` before use. No Node runtime or server is needed.

```sh
ob update --source https://github.com/boan-anbo/obc/releases/latest/download/cli-manifest.json
ob query --q "history of computing" --limit 20
ob query --period qing --format book --sort year-new
ob show --id 4500
```

Queries, show and status work offline. Updates are explicit. For local transfer,
use `ob update --bundle catalog.json.gz`. `--dataset FILE` reads a bundle in place;
`--cache DIR` selects a cache. Run `ob --help` for filters and options.
JSON is default; use `--output jsonl` for work records only. Preserve snapshot
while paging with offset. Cite original creators and titles with
`https://anbo.me/obc/works/ID/`.

Build from source with Go1.24+: `go build -o ob ./cmd/ob`. The only external module
is golang.org/x/text; its notice is retained. Maintainer reference exports require
the matching site source and its installed TS tooling. `scripts/build-release.sh`
builds six targets without installing or publishing. Pass `--asset-base` for
immutable HTTPS release URLs; otherwise it prepares a local static layout.

Code: MIT, copyright Bo An (`LICENSE`). Data: CC BY4.0 (`DATA-LICENSE`).

Data with work relations requires ob >= 0.3.0. Query/show JSON preserves the accepted
language, romanized titles/names and English title with its editorial flag.
These retained variants are searchable. This reader keeps its legacy lexical
policy; it does not claim browser Fuse parity. `relations` contains strict
`{type, work}` objects: `hasVersion` or `isVersionOf` and a public numeric ID.
Targets and reciprocal links belong to the same snapshot. Versions 0.2.x reject
this data before activating it and retain the previous cache; download 0.3.0 first.
