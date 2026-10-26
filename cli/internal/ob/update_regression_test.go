package ob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManifestRedirectUsesFinalHTTPSBase(t *testing.T) {
	data := fixtureBytes(t)
	m := fixtureManifest(t, data)
	m.URL = "catalog.json.gz"
	a := m.Release.Artifacts["catalog.json.gz"]
	a.URL = m.URL
	m.Release.Artifacts["catalog.json.gz"] = a
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/v1/manifest.json":
			json.NewEncoder(w).Encode(m)
		case "/releases/v1/catalog.json.gz":
			w.Write(data)
		default:
			t.Errorf("wrong final-origin path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer target.Close()
	original := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest/manifest.json" {
			t.Errorf("bundle resolved on original origin: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, target.URL+"/releases/v1/manifest.json", 302)
	}))
	defer original.Close()
	cache := t.TempDir()
	if _, e := Update(context.Background(), cache, original.URL+"/latest/manifest.json", "", original.Client()); e != nil {
		t.Fatal(e)
	}
	path, _, e := CurrentData(cache)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Load(path); e != nil {
		t.Fatal(e)
	}
}
func TestCancelledLocalUpdateDoesNotActivate(t *testing.T) {
	cache := t.TempDir()
	local := filepath.Join(t.TempDir(), "data.json")
	os.WriteFile(local, fixtureBytes(t), 0600)
	if _, e := Update(context.Background(), cache, "", local, nil); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(cache, "current.json"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Update(ctx, cache, "", local, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(cache, "current.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled update activated")
	}
	// This corpus takes much longer than 1ms to decode/validate. Cancellation may
	// happen during validation; it must still prevent the activation rename.
	if real := os.Getenv("OB_REAL_DATA"); real != "" {
		ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		if _, e := Update(ctx, cache, "", real, nil); !errors.Is(e, context.DeadlineExceeded) {
			t.Fatal(e)
		}
		after, _ = os.ReadFile(filepath.Join(cache, "current.json"))
		if !bytes.Equal(before, after) {
			t.Fatal("deadline expired before validation finished but data activated")
		}
	}
}
func TestAtomicWriterChecksCancellationBeforeRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.json")
	os.WriteFile(path, []byte("old"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := writeAtomic(ctx, dir, path, []byte("new")); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "old" {
		t.Fatal("cancelled commit overwrote pointer")
	}
}
func TestReleaseCompatibilityRejectsFutureReader(t *testing.T) {
	m := fixtureManifest(t, fixtureBytes(t))
	m.Release.MinimumReaderVersion = "9.0.0"
	if e := m.Validate(); e == nil {
		t.Fatal("future minimum reader accepted")
	}
	m.Release.MinimumReaderVersion = "0.0.1"
	if e := m.Validate(); e != nil {
		t.Fatal(e)
	}
	m.Release.SemanticsSHA256 = "bad"
	if e := m.Validate(); e == nil {
		t.Fatal("unknown semantics accepted")
	}
}

func TestNumericReaderVersions(t *testing.T) {
	for _, v := range []string{"0.0.0", "0.0.1", "0.1.0", "0.1.1", "0.2.0", "0.2.1", "0.3.0"} {
		if !supportsMinimum(v) {
			t.Fatal("supported minimum rejected", v)
		}
	}
	for _, v := range []string{"0.3.1", "1.0.0", "00.1.0", "garbage", "0\\a1\\b0"} {
		if supportsMinimum(v) {
			t.Fatal("unsupported minimum accepted", v)
		}
	}
}

func TestReleaseOverlayMappingMustMatchEmbeddedOverlay(t *testing.T) {
	data := fixtureBytes(t)
	manifest := fixtureManifest(t, data)
	if _, err := manifest.ValidateBundle(data); err != nil {
		t.Fatal(err)
	}
	manifest.Release.Overlay.MappingSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := manifest.Validate(); err != nil {
		t.Fatalf("regression requires valid metadata shape: %v", err)
	}
	if _, err := manifest.ValidateBundle(data); err == nil {
		t.Fatal("contradictory overlay mapping accepted")
	}
}
