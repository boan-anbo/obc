package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"openbibliographies/ob/internal/distribution"
	"openbibliographies/ob/internal/ob"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticHTTPSFixtureUpdateAndOfflineReopen(t *testing.T) {
	pkg := os.Getenv("OB_RELEASE_PACKAGE")
	if pkg == "" {
		t.Skip("OB_RELEASE_PACKAGE required")
	}
	root := filepath.Join(t.TempDir(), "static")
	if e := run(pkg, root); e != nil {
		t.Fatal(e)
	}
	p, e := distribution.Open(pkg)
	if e != nil {
		t.Fatal(e)
	}
	latest := filepath.Join(root, "cli", "latest.json")
	immutable := filepath.Join(root, "cli", "releases", p.Manifest.Release.ID, "cli-manifest.json")
	a, _ := os.ReadFile(latest)
	b, _ := os.ReadFile(immutable)
	if !bytes.Equal(a, b) {
		t.Fatal("latest differs from immutable manifest")
	}
	server := httptest.NewTLSServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	// The published inventory may use absolute GitHub asset URLs. Rebind only
	// this owned static fixture so the test never requests an external host.
	var fixture ob.Manifest
	if e = ob.Decode(a, &fixture); e != nil {
		t.Fatal(e)
	}
	for name, artifact := range fixture.Release.Artifacts {
		artifact.URL = server.URL + "/cli/releases/" + fixture.Release.ID + "/" + name
		fixture.Release.Artifacts[name] = artifact
	}
	fixture.URL = fixture.Release.Artifacts["catalog.json.gz"].URL
	fixtureBytes, e := json.Marshal(fixture)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(latest, fixtureBytes, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(immutable, fixtureBytes, 0600); e != nil {
		t.Fatal(e)
	}
	cache := t.TempDir()
	if _, e = ob.Update(context.Background(), cache, server.URL+"/cli/latest.json", "", server.Client()); e != nil {
		server.Close()
		t.Fatal(e)
	}
	server.Close()
	path, _, e := ob.CurrentData(cache)
	if e != nil {
		t.Fatal(e)
	}
	x, e := ob.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	q, _ := ob.Parse(map[string][]string{"q": {"Adams Bodomo"}})
	r, e := x.Execute(q)
	if e != nil || r.Matched != 5 {
		t.Fatal(e, r.Matched)
	}
}
