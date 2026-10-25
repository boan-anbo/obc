// Split reference-engine exports. This is a maintainer preparation tool.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"openbibliographies/ob/internal/ob"
	"os"
	"path/filepath"
)

func main() {
	export := flag.String("export", "reference/export.json", "real reference export JSON")
	fixture := flag.String("fixture", "reference/fixture-export.json", "synthetic fixture export JSON")
	out := flag.String("out", "reference", "output directory")
	flag.Parse()
	if e := run(*export, *fixture, *out); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(rp, fp, out string) error {
	b, e := ob.ReadFile(rp)
	if e != nil {
		return e
	}
	var real struct {
		Retrieval  ob.Retrieval
		Cases      json.RawMessage
		Sources    map[string]string
		Generation struct {
			SourceOrder  []string
			SourceHashes map[string]string
		}
	}
	if e = json.Unmarshal(b, &real); e != nil {
		return e
	}
	var sourceBytes bytes.Buffer
	if len(real.Generation.SourceOrder) != 11 {
		return fmt.Errorf("exact reference source closure is required")
	}
	for _, name := range real.Generation.SourceOrder {
		source, ok := real.Sources[name]
		if !ok || filepath.Base(name) != name || name == ".." {
			return fmt.Errorf("invalid reference input")
		}
		if ob.Digest([]byte(source)) != real.Generation.SourceHashes[name] {
			return fmt.Errorf("source hash mismatch: %s", name)
		}
		sourceBytes.WriteString(source)
	}
	if ob.Digest(sourceBytes.Bytes()) != real.Retrieval.SemanticsSHA256 {
		return fmt.Errorf("reference source closure digest mismatch")
	}
	if real.Retrieval.SemanticsSHA256 != ob.SemanticsSHA256 {
		return fmt.Errorf("reference semantics changed; port/review native engine before changing its supported digest")
	}
	if e = os.MkdirAll(out, 0755); e != nil {
		return e
	}
	snapshotDir := filepath.Join(out, "source-snapshot")
	if e = os.MkdirAll(snapshotDir, 0755); e != nil {
		return e
	}
	for _, name := range real.Generation.SourceOrder {
		if e = os.WriteFile(filepath.Join(snapshotDir, name), []byte(real.Sources[name]), 0644); e != nil {
			return e
		}
	}
	r, _ := json.Marshal(real.Retrieval)
	if e = os.WriteFile(filepath.Join(out, "retrieval.json"), r, 0644); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(out, "real-golden.json"), real.Cases, 0644); e != nil {
		return e
	}
	b, e = ob.ReadFile(fp)
	if e != nil {
		return e
	}
	var fixture struct {
		CatalogJSON string
		Retrieval   ob.Retrieval
		Cases       json.RawMessage
	}
	if e = json.Unmarshal(b, &fixture); e != nil {
		return e
	}
	fixture.Retrieval.SemanticsSHA256 = ob.SemanticsSHA256
	bundle := ob.Bundle{Schema: ob.BundleSchema, Snapshot: "fixture-v1", Development: true, Source: "synthetic test fixture", Catalog: fixture.CatalogJSON, Retrieval: fixture.Retrieval}
	if _, e = ob.Build(bundle); e != nil {
		return e
	}
	b, _ = json.Marshal(bundle)
	if e = os.WriteFile(filepath.Join(out, "fixture.json"), b, 0644); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(out, "fixture-golden.json"), fixture.Cases, 0644)
}
