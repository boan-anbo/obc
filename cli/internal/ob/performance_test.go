package ob

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

// Opt-in maintainer instrumentation; never runs in ordinary queries or tests.
func TestRealPhaseProfile(t *testing.T) {
	out := os.Getenv("OB_PHASE_OUTPUT")
	if out == "" {
		t.Skip("OB_PHASE_OUTPUT required")
	}
	path := os.Getenv("OB_REAL_DATA")
	if path == "" {
		t.Fatal("OB_REAL_DATA required")
	}
	if profile := os.Getenv("OB_CPU_PROFILE"); profile != "" {
		f, e := os.Create(profile)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if e = pprof.StartCPUProfile(f); e != nil {
			t.Fatal(e)
		}
		defer pprof.StopCPUProfile()
	}
	result := map[string]any{"note": "isolated instrumented phases; GC between stages; not process startup benchmark"}
	phase := func(name string, f func()) {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		f()
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		runtime.GC()
		var retained runtime.MemStats
		runtime.ReadMemStats(&retained)
		result[name] = map[string]any{"ms": float64(elapsed.Microseconds()) / 1000, "allocatedBytes": after.TotalAlloc - before.TotalAlloc, "retainedHeapBytesAfterGC": retained.HeapAlloc}
	}
	var compressed, raw []byte
	var bundle Bundle
	var x *Index
	var e error
	phase("readCompressed", func() {
		compressed, e = ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
	})
	phase("inflate", func() {
		z, e := gzip.NewReader(bytes.NewReader(compressed))
		if e != nil {
			t.Fatal(e)
		}
		defer z.Close()
		raw, e = ReadBounded(z, MaxData)
		if e != nil {
			t.Fatal(e)
		}
	})
	phase("decodeEnvelope", func() {
		if e = Decode(raw, &bundle); e != nil {
			t.Fatal(e)
		}
		raw = nil
		compressed = nil
	})
	phase("catalogValidationAndMetadataIndex", func() {
		x, e = Build(bundle)
		if e != nil {
			t.Fatal(e)
		}
	})
	phase("buildLexicalIndex", func() { x.buildSearch() })
	q, _ := Parse(url.Values{"q": {"history of computing"}, "limit": {"5"}})
	phase("queryWithBuiltIndex", func() {
		r, e := x.Execute(q)
		if e != nil || r.Matched != 6 {
			t.Fatal(e, r.Matched)
		}
	})
	// This standalone probe is included in Build above; it is not an additional
	// phase of the actual loader and must not be added to the stage totals.
	phase("catalogJSONDecodeProbeIncludedInBuild", func() {
		var c Catalog
		if e = Decode([]byte(bundle.Catalog), &c); e != nil {
			t.Fatal(e)
		}
		runtime.KeepAlive(c)
	})
	h := sha256.New()
	for _, tok := range x.Search.Vocabulary {
		fmt.Fprintf(h, "%s:%d\n", tok, x.Search.DF[tok])
		for _, p := range x.Search.Postings[tok] {
			fmt.Fprintf(h, "%d,%d,%d;", p.Doc, p.Field, p.TF)
		}
	}
	for f := 0; f < 4; f++ {
		fmt.Fprint(h, x.Search.Average[f])
		for _, n := range x.Search.Lengths[f] {
			fmt.Fprint(h, n, ",")
		}
	}
	result["postingsAndLengthsSHA256"] = fmt.Sprintf("%x", h.Sum(nil))
	runtime.KeepAlive(x)
	b, _ := json.MarshalIndent(result, "", "  ")
	if e = os.WriteFile(out, append(b, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
}
