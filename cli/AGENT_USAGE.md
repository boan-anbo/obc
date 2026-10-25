---
name: obc
description: Search and cite the Open Bibliographies for China Studies collection using its offline ob CLI.
---

Use `ob` to discover English scholarship about China and inspect its public bibliographic metadata and classifications. Prefer the local CLI to querying the website, which may be rate-limited.

Download the binary for the machine and `catalog.json.gz` from the same release at https://github.com/boan-anbo/obc/releases. Verify the supplied checksums, make the binary executable where required, and place it on PATH. Load the bundle once:

```sh
ob update --bundle /path/to/catalog.json.gz
ob query --q "history of computing" --limit 20
ob show --id 4500
```

Queries run offline. Use `--discipline`, `--region`, `--period`, `--subject`, `--topic`, `--format`, or `--workType` with exact term IDs; comma-separated choices are OR within a filter, and different filters combine with AND. `--author` takes an author key. Run `ob --help` for available options.

JSON is the default. Preserve the returned snapshot while paginating with `--offset`; use `--output jsonl` when only work records are needed. Missing metadata stays unknown.

Cite the recorded creator, title and publication details, linking the stable work number as `https://anbo.me/obc/works/ID/`. Source classifications describe the work; publication year is not the period studied.
