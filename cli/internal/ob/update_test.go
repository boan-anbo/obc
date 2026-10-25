package ob

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../testdata/fixture.json")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func fixtureIndex(t *testing.T) *Index {
	t.Helper()
	x, e := LoadBytes(fixtureBytes(t))
	if e != nil {
		t.Fatal(e)
	}
	return x
}
func fixtureManifest(t *testing.T, b []byte) Manifest {
	t.Helper()
	x, e := LoadBytes(b)
	if e != nil {
		t.Fatal(e)
	}
	m := Manifest{ManifestSchema, x.Bundle.Snapshot, true, "/data", Digest(b), int64(len(b)), BundleSchema, x.Bundle.Retrieval.CatalogSHA256, x.Bundle.Retrieval.AuthorVersion, len(x.Catalog.Works), nil}
	raw, e := Inflate(b)
	if e != nil {
		t.Fatal(e)
	}
	var o AuthorOverlay
	Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &o)
	m.Release = &Release{SemanticsSHA256: SemanticsSHA256, ID: "fixture-v1", CLI: CLIIdentity{"fixture", "sha256:" + strings.Repeat("a", 64), strings.Repeat("a", 64), ReaderVersion}, MinimumReaderVersion: ReaderVersion, ReaderContract: ReaderContract, Generated: x.Catalog.Generated, ClassificationVersion: x.Catalog.ClassificationVersion, CatalogSchema: x.Catalog.Schema, CatalogSize: int64(len(x.Bundle.Catalog)), UncompressedBundle: ByteIdentity{int64(len(raw)), Digest(raw)}, Overlay: OverlayIdentity{o.Schema, o.Snapshot, o.CatalogSHA256, o.Version, o.MappingSHA256}, Artifacts: map[string]Artifact{"catalog.json.gz": {URL: m.URL, Size: m.Size, SHA256: m.SHA256, Kind: "bundle"}, "author-aggregation.json": {URL: "/overlay", Size: int64(len(x.Bundle.Retrieval.OverlayJSON)), SHA256: x.Bundle.Retrieval.OverlaySHA256, Kind: "author-overlay"}}, Licenses: ReleaseLicenses()}
	return m
}
func TestUpdatesRetainOldOnFailure(t *testing.T) {
	good := fixtureBytes(t)
	cache := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "bundle.json")
	os.WriteFile(bundle, good, 0600)
	if _, e := Update(context.Background(), cache, "", bundle, nil); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(cache, "current.json"))
	m := fixtureManifest(t, good)
	for _, mode := range []string{"partial", "hash", "oversized", "schema", "count", "snapshot", "timeout", "http-data", "redirect-http", "unknown-manifest", "wrong-version"} {
		t.Run(mode, func(t *testing.T) {
			manifest := m
			releaseCopy := *m.Release
			releaseCopy.Artifacts = maps.Clone(m.Release.Artifacts)
			manifest.Release = &releaseCopy
			data := good
			switch mode {
			case "hash":
				manifest.SHA256 = strings.Repeat("a", 64)
			case "oversized":
				data = append(bytes.Clone(good), 'x')
			case "schema":
				data = []byte(`{"schema":"wrong"}`)
				manifest.Size = int64(len(data))
				manifest.SHA256 = Digest(data)
			case "count":
				manifest.Works++
			case "snapshot":
				manifest.Snapshot = "different"
			case "http-data":
				manifest.URL = "http://127.0.0.1/data"
			case "wrong-version":
				manifest.BundleSchema = "future"
			}
			a := manifest.Release.Artifacts["catalog.json.gz"]
			a.URL, a.SHA256, a.Size = manifest.URL, manifest.SHA256, manifest.Size
			manifest.Release.Artifacts["catalog.json.gz"] = a
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/manifest":
					if mode == "redirect-http" {
						http.Redirect(w, r, "http://127.0.0.1/unsafe", 302)
						return
					}
					if mode == "unknown-manifest" {
						w.Write([]byte(`{"schema":"openbibliographies.data-release.v1"}`))
						return
					}
					json.NewEncoder(w).Encode(manifest)
				case "/data":
					if mode == "timeout" {
						select {
						case <-r.Context().Done():
							return
						case <-time.After(time.Second):
							return
						}
					}
					if mode == "partial" {
						w.Header().Set("Content-Length", strconv.Itoa(len(data)))
						w.Write(data[:len(data)/2])
						return
					}
					w.Write(data)
				}
			}))
			defer server.Close()
			client := server.Client()
			client.CheckRedirect = HTTPClient().CheckRedirect
			client.Timeout = 100 * time.Millisecond
			if _, e := Update(context.Background(), cache, server.URL+"/manifest", "", client); e == nil {
				t.Fatal("expected failure")
			}
			after, _ := os.ReadFile(filepath.Join(cache, "current.json"))
			if !bytes.Equal(before, after) {
				t.Fatal("failed update changed active cache")
			}
			path, _, e := CurrentData(cache)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = Load(path); e != nil {
				t.Fatalf("old data unusable: %v", e)
			}
		})
	}
	// A successful HTTPS update commits the new pointer and remains usable without the server.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest" {
			json.NewEncoder(w).Encode(m)
		} else {
			w.Write(good)
		}
	}))
	if _, e := Update(context.Background(), cache, server.URL+"/manifest", "", server.Client()); e != nil {
		t.Fatal(e)
	}
	server.Close()
	path, _, e := CurrentData(cache)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Load(path); e != nil {
		t.Fatal(e)
	}
	files, _ := filepath.Glob(filepath.Join(cache, ".incoming-*"))
	if len(files) != 0 {
		t.Fatal("partial files remain")
	}
}
func TestConcurrentUpdatesAndReaders(t *testing.T) {
	b := fixtureBytes(t)
	cache := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "fixture.json")
	os.WriteFile(bundle, b, 0600)
	if _, e := Update(context.Background(), cache, "", bundle, nil); e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	m := fixtureManifest(t, b)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest" {
			close(entered)
			<-release
			json.NewEncoder(w).Encode(m)
		} else {
			w.Write(b)
		}
	}))
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		_, e := Update(context.Background(), cache, server.URL+"/manifest", "", server.Client())
		done <- e
	}()
	<-entered
	if _, e := Update(context.Background(), cache, "", bundle, nil); e == nil {
		t.Fatal("concurrent update accepted")
	} else {
		var oe *Error
		if !errors.As(e, &oe) || oe.Exit != 5 {
			t.Fatal(e)
		}
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				path, _, e := CurrentData(cache)
				if e != nil {
					t.Error(e)
					return
				}
				if _, e = Load(path); e != nil {
					t.Error(e)
					return
				}
			}
		}()
	}
	close(release)
	wg.Wait()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestSchemaAndOverlayFailures(t *testing.T) {
	original := fixtureIndex(t).Bundle
	mutate := func(name string, f func(*Bundle)) {
		t.Run(name, func(t *testing.T) {
			b := original
			f(&b)
			if _, e := Build(b); e == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
	mutate("semantics", func(b *Bundle) { b.Retrieval.SemanticsSHA256 = strings.Repeat("0", 64) })
	mutate("catalog-hash", func(b *Bundle) { b.Catalog += " " })
	mutate("order", func(b *Bundle) { b.Retrieval.TitleOrder = make([]int, len(b.Retrieval.TitleOrder)) })
	for _, mode := range []string{"snapshot", "catalog", "stale", "conflict", "hash"} {
		mutate("overlay-"+mode, func(b *Bundle) {
			var o AuthorOverlay
			json.Unmarshal([]byte(b.Retrieval.OverlayJSON), &o)
			switch mode {
			case "snapshot":
				o.Snapshot = "wrong"
			case "catalog":
				o.CatalogSHA256 = strings.Repeat("0", 64)
			case "stale":
				o.Groups = []AuthorGroup{{Key: "person-fixture", Name: "Fixture", Members: []Member{{"missing", "Who", "author"}}}}
			case "conflict":
				g := AuthorGroup{Key: "person-fixture", Name: "Fixture", Members: []Member{{"computer", "Alice Example", "author"}, {"computer", "Alice Example", "author"}}}
				o.Groups = []AuthorGroup{g}
			}
			bytes, _ := json.Marshal(o)
			b.Retrieval.OverlayJSON = string(bytes)
			b.Retrieval.OverlaySHA256 = Digest(bytes)
			if mode == "hash" {
				b.Retrieval.OverlayJSON += " "
			}
		})
	}
	var zip bytes.Buffer
	gz := gzip.NewWriter(&zip)
	gz.Write(bytes.Repeat([]byte("x"), MaxData+1))
	gz.Close()
	if _, e := LoadBytes(zip.Bytes()); e == nil {
		t.Fatal("gzip bomb accepted")
	}
}
func TestGenerationsRemainAndReadOnly(t *testing.T) {
	cache := t.TempDir()
	b := fixtureBytes(t)
	local := filepath.Join(t.TempDir(), "data.json")
	os.WriteFile(local, b, 0600)
	first, e := Update(context.Background(), cache, "", local, nil)
	if e != nil {
		t.Fatal(e)
	}
	x := fixtureIndex(t)
	x.Bundle.Source = "second fixture source"
	b, _ = json.Marshal(x.Bundle)
	os.WriteFile(local, b, 0600)
	second, e := Update(context.Background(), cache, "", local, nil)
	if e != nil {
		t.Fatal(e)
	}
	if first.Manifest.SHA256 == second.Manifest.SHA256 {
		t.Fatal("fixture did not change")
	}
	for _, h := range []string{first.Manifest.SHA256, second.Manifest.SHA256} {
		if _, e := Load(filepath.Join(cache, h+".json.gz")); e != nil {
			t.Fatal(e)
		}
	}
}
