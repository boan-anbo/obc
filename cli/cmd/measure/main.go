// Measure release process starts and in-process queries. Does not flush OS caches.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"openbibliographies/ob/internal/ob"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"
)

func main() {
	bin := flag.String("bin", "dist/release/ob_darwin_arm64", "native release binary")
	data := flag.String("dataset", "dist/catalog.json.gz", "bundle")
	n := flag.Int("n", 5, "process repetitions")
	flag.Parse()
	if *n < 1 {
		panic("n must be positive")
	}
	out := map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "processRepetitions": *n, "cacheNote": "new processes; OS filesystem cache is warm/uncontrolled, not a disk-cold claim"}
	for _, scenario := range []struct{ Name, Q string }{{"filter-only", ""}, {"full-author", "Adams Bodomo"}, {"lexical-intent", "history of computing"}} {
		times := []float64{}
		var matched int
		for i := 0; i < *n; i++ {
			start := time.Now()
			b, e := exec.Command(*bin, "query", "--dataset", *data, "--q", scenario.Q, "--limit", "5").Output()
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				os.Exit(1)
			}
			var r ob.Result
			if e = json.Unmarshal(b, &r); e != nil {
				panic(e)
			}
			matched = r.Matched
			times = append(times, float64(time.Since(start).Microseconds())/1000)
		}
		sort.Float64s(times)
		out[scenario.Name+"ProcessMs"] = map[string]any{"min": times[0], "median": times[len(times)/2], "max": times[len(times)-1], "matched": matched}
	}
	start := time.Now()
	x, e := ob.Load(*data)
	if e != nil {
		panic(e)
	}
	out["loadIndexMs"] = float64(time.Since(start).Microseconds()) / 1000
	for _, scenario := range []struct{ Name, Q string }{{"filter-only", ""}, {"full-author", "Adams Bodomo"}, {"lexical-intent", "history of computing"}} {
		q, _ := ob.Parse(url.Values{"q": {scenario.Q}, "limit": {"5"}})
		x.Execute(q)
		start = time.Now()
		for i := 0; i < 20; i++ {
			if _, e = x.Execute(q); e != nil {
				panic(e)
			}
		}
		out[scenario.Name+"WarmMeanMs"] = float64(time.Since(start).Microseconds()) / 20000
	}
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	out["retainedHeapBytes"] = m.HeapAlloc
	out["heapReservedBytes"] = m.HeapSys
	runtime.KeepAlive(x)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
