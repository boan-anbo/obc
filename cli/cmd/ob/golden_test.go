package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"testing"

	"openbibliographies/ob/internal/ob"
)

// Run by hand on a release bundle: OB_BUNDLE=... OB_GOLDEN=... go test -run Golden -v.
// Ranking is not shared with the site, so this reports recall; it does not pin it.
func TestGoldenRecall(t *testing.T) {
	bundle, path := os.Getenv("OB_BUNDLE"), os.Getenv("OB_GOLDEN")
	if bundle == "" || path == "" {
		t.Skip("OB_BUNDLE and OB_GOLDEN not set")
	}
	x, e := ob.Load(bundle)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var g struct {
		Queries []struct {
			Q, Lang, Kind, Origin, Mode string
			Expect                      []string
		}
	}
	if e = json.Unmarshal(raw, &g); e != nil {
		t.Fatal(e)
	}
	type tally struct{ n, at1, at10 int }
	totals := map[string]*tally{}
	add := func(key string, at1, at10 bool) {
		if totals[key] == nil {
			totals[key] = &tally{}
		}
		totals[key].n++
		if at1 {
			totals[key].at1++
		}
		if at10 {
			totals[key].at10++
		}
	}
	for _, c := range g.Queries {
		q, e := ob.Parse(url.Values{"q": {c.Q}, "limit": {"100"}})
		if e != nil {
			t.Fatal(c.Q, e)
		}
		r, e := x.Execute(q)
		if e != nil {
			t.Fatal(c.Q, e)
		}
		ids := []string{}
		for _, w := range r.Works {
			ids = append(ids, w.ID)
		}
		top := ids[:min(10, len(ids))]
		at1 := len(ids) > 0 && slices.Contains(c.Expect, ids[0])
		at10 := false
		for _, id := range c.Expect {
			if slices.Contains(top, id) {
				at10 = true
			} else if c.Mode == "all" {
				at10 = false
				break
			}
		}
		for _, key := range []string{"ALL", "origin:" + c.Origin, "lang:" + c.Lang, "kind:" + c.Kind} {
			add(key, at1, at10)
		}
		if !at10 {
			fmt.Printf("MISS %s [%s/%s] got %v want %v\n", c.Q, c.Lang, c.Kind, top[:min(3, len(top))], c.Expect[:min(3, len(c.Expect))])
		}
	}
	keys := []string{}
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := totals[k]
		fmt.Printf("%-24s n=%3d r@1 %5.1f%% r@10 %5.1f%%\n", k, v.n, 100*float64(v.at1)/float64(v.n), 100*float64(v.at10)/float64(v.n))
	}
}
