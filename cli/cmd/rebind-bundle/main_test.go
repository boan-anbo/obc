package main

import (
	"encoding/json"
	"openbibliographies/ob/internal/ob"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentReviewedOverlayIsBoundAndRetained(t *testing.T) {
	base := "../../testdata/fixture.json"
	x, e := ob.Load(base)
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	catalog := filepath.Join(root, "catalog.json")
	raw, e := json.Marshal(x.Catalog)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(catalog, raw, 0600)
	order := filepath.Join(root, "order.json")
	b, _ := json.Marshal(x.Bundle.Retrieval.TitleOrder)
	os.WriteFile(order, b, 0600)
	var o ob.AuthorOverlay
	if e = ob.Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &o); e != nil {
		t.Fatal(e)
	}
	o.CatalogSHA256 = ob.Digest(raw)
	o.Snapshot = "dev-" + o.CatalogSHA256[:16]
	o.Catalog = "/data/catalog/" + o.CatalogSHA256[:16] + ".json"
	o.MappingSHA256 = strings.Repeat("b", 64)
	o.Version = "reviewed-authors-20261026-" + o.MappingSHA256
	overlay := filepath.Join(root, "overlay.json")
	b, _ = json.Marshal(o)
	os.WriteFile(overlay, b, 0600)
	out := filepath.Join(root, "new.json.gz")
	if e = run(base, catalog, order, out, overlay); e != nil {
		t.Fatal(e)
	}
	next, e := ob.Load(out)
	if e != nil || next.Bundle.Retrieval.AuthorVersion != o.Version {
		t.Fatal("old author overlay retained", e)
	}
	var got ob.AuthorOverlay
	if e = ob.Decode([]byte(next.Bundle.Retrieval.OverlayJSON), &got); e != nil || got.MappingSHA256 != o.MappingSHA256 {
		t.Fatal(got, e)
	}
	o.CatalogSHA256 = strings.Repeat("0", 64)
	b, _ = json.Marshal(o)
	os.WriteFile(overlay, b, 0600)
	bad := filepath.Join(root, "bad.json.gz")
	if e = run(base, catalog, order, bad, overlay); e == nil {
		t.Fatal("stale overlay accepted")
	}
	if _, e = os.Stat(bad); !os.IsNotExist(e) {
		t.Fatal("bad output created")
	}
}
