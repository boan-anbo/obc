# Open Bibliographies for China Studies

A bibliography of 20,905 works, with classifications and an offline search CLI.

Data is in `data/`: `works.jsonl` and `works.csv` contain the same records;
`vocabulary.json` describes the terms; `schema.json` defines the fields;
`manifest.json` and `SHA256SUMS` bind the snapshot and files.

Fields: `id`, `title`, `creators` (name, role, optional romanized), `year`,
`venue`, `format`, `workType`, `labels` (discipline, region, period, subject,
topic, keyword), `isbn`, `doi`; for non-English works also `language`,
`titleRomanized` and `titleEnglish` (value, and whether it is our translation);
`relations` links versions of a work in other languages by ID.
IDs are stable number strings. Missing metadata stays missing;
publication year is separate from the period studied.

## CLI

Download the matching binary and data bundle from [Releases](https://github.com/boan-anbo/obc/releases).
Verify the checksums and place the binary on PATH as `ob` (`ob.exe` on Windows).

The current data needs `ob` 0.3.0 or later. Older versions refuse it and keep
their existing data; download the latest `ob` from the latest release.

```sh
ob update --source https://github.com/boan-anbo/obc/releases/latest/download/cli-manifest.json
ob query --q "history of computing" --limit 20
ob show --id 4500
```

For offline installation, download `catalog.json.gz` and run `ob update --bundle FILE`.
Go source is in `cli/`; build with `go build ./cmd/ob`. See `SKILL.md` for agent use.

Data and classifications: **CC BY 4.0**, full text in `data/LICENSE`.
CLI code: **MIT**, copyright Bo An, in `cli/LICENSE`.

Attribution: Open Bibliographies for China Studies, curated by Bo An, https://anbo.me/obc/
