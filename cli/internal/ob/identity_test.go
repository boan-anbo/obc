package ob

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

type identityWork struct {
	ID, Title, TitleRomanized, TitleEnglish, Language string
	Names                                             []string
}
type identityCase struct {
	Query            string
	Titles, Creators []string
	Order            []string
}

// identityIndex builds an ordinary index over the shared snapshot-free catalogue, so the
// cases run through the same load and query path as a release bundle.
func identityIndex(t *testing.T, works []identityWork) *Index {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join("..", "..", "testdata", "fixture.json"))
	if e != nil {
		t.Fatal(e)
	}
	var bundle Bundle
	if e = json.Unmarshal(raw, &bundle); e != nil {
		t.Fatal(e)
	}
	var c map[string]any
	if e = json.Unmarshal([]byte(bundle.Catalog), &c); e != nil {
		t.Fatal(e)
	}
	list := []any{}
	order := []int{}
	for i, w := range works {
		creators := []any{}
		for _, name := range w.Names {
			creators = append(creators, map[string]any{"name": name, "role": "author"})
		}
		labels := map[string]any{}
		for _, d := range Dimensions {
			labels[d] = []string{}
		}
		row := map[string]any{"id": w.ID, "title": w.Title, "creators": creators, "format": "book", "workType": []string{"study"}, "labels": labels}
		if w.TitleRomanized != "" {
			row["titleRomanized"] = w.TitleRomanized
		}
		if w.TitleEnglish != "" {
			row["titleEnglish"] = map[string]any{"value": w.TitleEnglish, "editorial": false}
		}
		if w.Language != "" {
			row["language"] = w.Language
		}
		list = append(list, row)
		order = append(order, i)
	}
	sort.Slice(order, func(i, j int) bool { return works[order[i]].Title < works[order[j]].Title })
	c["works"] = list
	coverage := c["coverage"].(map[string]any)
	coverage["shown"], coverage["rows"] = len(list), len(list)
	body, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	bundle.Catalog = string(body)
	bundle.Retrieval.CatalogSHA256 = Digest(body)
	bundle.Retrieval.TitleOrder = order
	var overlay map[string]any
	if e = json.Unmarshal([]byte(bundle.Retrieval.OverlayJSON), &overlay); e != nil {
		t.Fatal(e)
	}
	overlay["catalogSha256"] = bundle.Retrieval.CatalogSHA256
	overlay["catalog"] = "/data/catalog/" + bundle.Retrieval.CatalogSHA256[:16] + ".json"
	again, e := json.Marshal(overlay)
	if e != nil {
		t.Fatal(e)
	}
	bundle.Retrieval.OverlayJSON = string(again)
	bundle.Retrieval.OverlaySHA256 = Digest(again)
	x, e := Build(bundle)
	if e != nil {
		t.Fatal(e)
	}
	return x
}

func TestSharedIdentityVectors(t *testing.T) {
	raw, e := os.ReadFile(filepath.Join("testdata", "identity.v1.json"))
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Works []identityWork
		Cases []identityCase
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	if len(v.Cases) != 21 {
		t.Fatal(len(v.Cases))
	}
	x := identityIndex(t, v.Works)
	for _, c := range v.Cases {
		titles, creators := []string{}, []string{}
		for doc, drop := range x.identityHits(c.Query) {
			if drop == titleHit {
				titles = append(titles, v.Works[doc].ID)
			} else {
				creators = append(creators, v.Works[doc].ID)
			}
		}
		slices.Sort(titles)
		slices.Sort(creators)
		slices.Sort(c.Titles)
		slices.Sort(c.Creators)
		if !slices.Equal(titles, c.Titles) || !slices.Equal(creators, c.Creators) {
			t.Errorf("%q: titles %v creators %v, want %v %v", c.Query, titles, creators, c.Titles, c.Creators)
		}
		if c.Query == "" || len(titles)+len(creators) == 0 && c.Order == nil {
			continue
		}
		q, e := Parse(url.Values{"q": {c.Query}, "limit": {"100"}})
		if e != nil {
			t.Fatal(e)
		}
		r, e := x.Execute(q)
		if e != nil {
			t.Fatal(c.Query, e)
		}
		got := []string{}
		for _, w := range r.Works {
			got = append(got, w.ID)
		}
		for _, id := range append(slices.Clone(c.Titles), c.Creators...) {
			if !slices.Contains(got, id) {
				t.Errorf("%q: hit %s missing from %v", c.Query, id, got)
			}
		}
		for i := 0; i+1 < len(c.Order); i++ {
			if a, b := slices.Index(got, c.Order[i]), slices.Index(got, c.Order[i+1]); a < 0 || b < 0 || a > b {
				t.Errorf("%q: order %v, got %v", c.Query, c.Order, got)
			}
		}
		rank := len(c.Titles) + len(c.Creators)
		for i, id := range got[:min(rank, len(got))] {
			if !slices.Contains(c.Titles, id) && !slices.Contains(c.Creators, id) {
				t.Errorf("%q: non-hit %s at %d in %v", c.Query, id, i, got)
			}
		}
	}
}
