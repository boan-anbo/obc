// Data-only maintainer preparation. Reuses the existing reader and requires an
// explicit title-order export; it does not generate classifications or queries.
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
	base := flag.String("base", "", "existing accepted bundle")
	catalog := flag.String("catalog", "", "new accepted catalog")
	order := flag.String("title-order", "", "normal title-order row indices")
	overlay := flag.String("author-overlay", "", "optional current reviewed public author overlay")
	out := flag.String("out", "", "create-only output bundle")
	flag.Parse()
	if e := run(*base, *catalog, *order, *out, *overlay); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(base, catalog, order, out, overlayPath string) error {
	x, e := ob.Load(base)
	if e != nil {
		return e
	}
	c, e := ob.ReadFile(catalog)
	if e != nil {
		return e
	}
	var indices []int
	b, e := ob.ReadFile(order)
	if e != nil {
		return e
	}
	if e = ob.Decode(b, &indices); e != nil {
		return e
	}
	var o ob.AuthorOverlay
	if e = ob.Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &o); e != nil {
		return e
	}
	sha := ob.Digest(c)
	snapshot := "dev-" + sha[:16]
	if overlayPath != "" {
		raw, e := ob.ReadFile(overlayPath)
		if e != nil {
			return e
		}
		if e = ob.Decode(raw, &o); e != nil {
			return e
		}
		if o.CatalogSHA256 != sha || o.Snapshot != snapshot || o.Version == "" {
			return fmt.Errorf("reviewed overlay is not bound to this catalog")
		}
	}
	o.Snapshot = snapshot
	o.CatalogSHA256 = sha
	o.Catalog = "/data/catalog/" + sha[:16] + ".json"
	overlay, e := json.Marshal(o)
	if e != nil {
		return e
	}
	bundle := x.Bundle
	bundle.Catalog = string(c)
	bundle.Snapshot = snapshot
	bundle.Retrieval.CatalogSHA256 = sha
	bundle.Retrieval.TitleOrder = indices
	bundle.Retrieval.AuthorVersion = o.Version
	bundle.Retrieval.OverlayJSON = string(overlay)
	bundle.Retrieval.OverlaySHA256 = ob.Digest(overlay)
	if _, e = ob.Build(bundle); e != nil {
		return e
	}
	f, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	z, e := gzip.NewWriterLevel(f, gzip.BestCompression)
	if e != nil {
		return e
	}
	if e = json.NewEncoder(z).Encode(bundle); e != nil {
		return e
	}
	if e = z.Close(); e != nil {
		return e
	}
	return f.Sync()
}
