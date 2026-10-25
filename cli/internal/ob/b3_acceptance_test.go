package ob

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func TestB3ActualAcceptedPublicVariants(t *testing.T) {
	path := os.Getenv("OB_B3_DATA")
	if path == "" {
		t.Skip("set OB_B3_DATA to pinned isolated B3 bundle")
	}
	x, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if x.Bundle.Retrieval.CatalogSHA256 != "c18093dee9db826f62856c893a8a1e76042f6e5533f1447021ae9e061be5f870" || len(x.Catalog.Works) != 20903 {
		t.Fatal("wrong actual B3 cohort")
	}
	probes := []map[string]any{}
	for _, w := range x.Catalog.Works {
		if w.ID != "22352" && w.ID != "22353" && w.ID != "22354" {
			continue
		}
		queries := workTitles(w)
		for _, c := range w.Creators {
			queries = appendUnique(queries, c.Name)
			if c.Romanized != "" {
				queries = appendUnique(queries, c.Romanized)
			}
		}
		for _, text := range queries {
			q, e := Parse(url.Values{"q": {text}, "limit": {"100"}})
			if e != nil {
				t.Fatal(e)
			}
			r, e := x.Execute(q)
			if e != nil {
				t.Fatal(e)
			}
			rank := 0
			for i, found := range r.Works {
				if found.ID == w.ID {
					rank = i + 1
				}
			}
			probes = append(probes, map[string]any{"id": w.ID, "query": text, "rank_within_first100": rank, "matched": r.Matched})
			if rank == 0 {
				t.Errorf("accepted public variant absent: %s %q", w.ID, text)
			}
		}
	}
	if out := os.Getenv("OB_B3_PROOF"); out != "" {
		raw, e := json.MarshalIndent(map[string]any{"catalog_sha256": x.Bundle.Retrieval.CatalogSHA256, "works": len(x.Catalog.Works), "probes": probes, "scope": "known public title/name variants in retained legacy lexical reader; not Fuse parity"}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		f, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(append(raw, '\n')); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
	}
}
