package ob

import (
	"encoding/json"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestFixtureConformance(t *testing.T) {
	x := fixtureIndex(t)
	b, e := os.ReadFile("../../testdata/fixture-golden.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Params map[string]string
		Result json.RawMessage
	}
	json.Unmarshal(b, &cases)
	for _, c := range cases {
		p := url.Values{}
		for k, v := range c.Params {
			p.Set(k, v)
		}
		t.Run(p.Encode(), func(t *testing.T) {
			q, e := Parse(p)
			if e != nil {
				t.Fatal(e)
			}
			r, e := x.Execute(q)
			if e != nil {
				t.Fatal(e)
			}
			got, _ := json.Marshal(r)
			var a, b any
			json.Unmarshal(got, &a)
			json.Unmarshal(c.Result, &b)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("fixture differs from canonical TypeScript query:\n%s\n%s", got, c.Result)
			}
		})
	}
}
func TestQueryValidationAndSnapshot(t *testing.T) {
	for _, s := range []string{"q=a&q=b", "limit=0", "limit=101", "offset=-1", "offset=1.5", "offset=9007199254740992", "wat=1", "keyword=typing", "sort=wrong", "output=markdown", "facets=keyword", "snapshot=bad_"} {
		p, _ := url.ParseQuery(s)
		if _, e := Parse(p); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
	for _, s := range []string{strings.Repeat("🌿", 513), "term0 term1 term2 term3 term4 term5 term6 term7 term8 term9 term10 term11 term12 term13 term14 term15 term16 term17 term18 term19 term20 term21 term22 term23 term24 term25 term26 term27 term28 term29 term30"} {
		if _, e := Parse(url.Values{"q": {s}}); e == nil {
			t.Fatal("query limit missing")
		}
	}
	x := fixtureIndex(t)
	for _, p := range []url.Values{{"author": {"missing"}}, {"format": {"markdown"}}, {"subject": {"missing"}}, {"snapshot": {"different"}}} {
		q, e := Parse(p)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = x.Execute(q); e == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	q, e := Parse(url.Values{"q": {strings.Repeat("🌿", 512)}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.Execute(q); e != nil {
		t.Fatal(e)
	}
}
func TestDatasetValidation(t *testing.T) {
	for _, mode := range []string{"cycle", "unknown-label", "duplicate", "count", "unknown-parent", "navigation"} {
		t.Run(mode, func(t *testing.T) {
			bundle := fixtureIndex(t).Bundle
			var c Catalog
			json.Unmarshal([]byte(bundle.Catalog), &c)
			switch mode {
			case "cycle":
				c.Taxonomy["period"][0].Parents = []string{c.Taxonomy["period"][0].ID}
			case "unknown-label":
				c.Works[0].Labels["subject"] = []string{"missing"}
			case "duplicate":
				c.Works[1].ID = c.Works[0].ID
			case "count":
				c.Coverage["shown"]++
			case "unknown-parent":
				c.Taxonomy["topic"][0].Parents = []string{"subject:missing"}
			case "navigation":
				c.Taxonomy["topic"][0].Navigation = []string{"subject:missing"}
			}
			bytes, _ := json.Marshal(c)
			bundle.Catalog = string(bytes)
			bundle.Retrieval.CatalogSHA256 = Digest(bytes)
			var overlay AuthorOverlay
			json.Unmarshal([]byte(bundle.Retrieval.OverlayJSON), &overlay)
			overlay.CatalogSHA256 = bundle.Retrieval.CatalogSHA256
			overlay.Catalog = "/data/catalog/" + overlay.CatalogSHA256[:16] + ".json"
			bytes, _ = json.Marshal(overlay)
			bundle.Retrieval.OverlayJSON = string(bytes)
			bundle.Retrieval.OverlaySHA256 = Digest(bytes)
			if _, e := Build(bundle); e == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}
