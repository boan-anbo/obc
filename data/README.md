# Open Bibliographies for China Studies

Compiled bibliography and classifications: 20905 works, snapshot `dev-ca097f53a30d11cc`.

- `works.jsonl`: one work per line; `works.csv`: the same records, with arrays encoded as JSON cells.
- `vocabulary.json`: accepted terms and their relationships.
- `schema.json`: record and vocabulary field definitions.
- `manifest.json` and `SHA256SUMS`: snapshot, counts and file hashes.
- `LICENSE`: Creative Commons Attribution 4.0 International legal text.

Fields: `id`, `title`, `creators` (name, role, optional romanized), `year`, `venue`, `format`, `workType`, `labels` (discipline, region, period, subject, topic, keyword), `isbn`, `doi`. Optional multilingual fields: `language`, `titleRomanized`, `titleEnglish` (value and editorial flag), and creator `romanized`. Missing values remain missing. IDs are stable decimal number strings; publication year is separate from the period studied.

Data license: CC BY 4.0 — https://creativecommons.org/licenses/by/4.0/
Attribution: Open Bibliographies for China Studies, curated by Bo An, https://anbo.me/obc/

Releases: https://github.com/boan-anbo/obc/releases

Optional `relations` contain `{type, work}` links using public numeric IDs. Types are `hasVersion` and `isVersionOf`; targets and reciprocal links are included in the same snapshot. CSV encodes this array as a JSON cell.
