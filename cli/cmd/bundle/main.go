// Maintainer tool: packages already generated public data; never regenerates a catalog.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"openbibliographies/ob/internal/ob"
	"os"
)

func main() {
	catalog := flag.String("catalog", "", "existing public catalog JSON")
	retrieval := flag.String("retrieval", "", "reference retrieval JSON")
	out := flag.String("out", "", "new .json.gz output")
	snapshot := flag.String("snapshot", "", "pinned snapshot")
	source := flag.String("source", "", "public source/provenance")
	flag.Parse()
	if e := run(*catalog, *retrieval, *out, *snapshot, *source); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(cp, rp, out, snapshot, source string) error {
	c, e := ob.ReadFile(cp)
	if e != nil {
		return e
	}
	r, e := ob.ReadFile(rp)
	if e != nil {
		return e
	}
	var retrieval ob.Retrieval
	if e = ob.Decode(r, &retrieval); e != nil {
		return e
	}
	b := ob.Bundle{Schema: ob.BundleSchema, Snapshot: snapshot, Development: true, Source: source, Catalog: string(c), Retrieval: retrieval}
	if _, e = ob.Build(b); e != nil {
		return e
	}
	f, e := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(out)
		}
	}()
	z, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	z.Name = ""
	if e = json.NewEncoder(z).Encode(b); e != nil {
		return e
	}
	if e = z.Close(); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	ok = true
	return f.Close()
}
