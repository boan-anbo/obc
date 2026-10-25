package ob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const ManifestSchema = "openbibliographies.cli-update.v1"

// This is a new release contract, separate from the website downloads manifest.
type Manifest struct {
	Schema        string   `json:"schema"`
	Snapshot      string   `json:"snapshot"`
	Development   bool     `json:"development"`
	URL           string   `json:"url"`
	SHA256        string   `json:"sha256"`
	Size          int64    `json:"size"`
	BundleSchema  string   `json:"bundleSchema"`
	CatalogSHA256 string   `json:"catalogSHA256"`
	AuthorVersion string   `json:"authorVersion"`
	Works         int      `json:"works"`
	Release       *Release `json:"release,omitempty"`
}
type Current struct {
	Manifest Manifest `json:"manifest"`
	Updated  string   `json:"updated"`
	Source   string   `json:"source"`
}

func DefaultCache() (string, error) {
	d, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "openbibliographies"), nil
}
func CurrentData(cache string) (string, *Current, error) {
	b, e := ReadFile(filepath.Join(cache, "current.json"))
	if errors.Is(e, os.ErrNotExist) {
		return "", nil, &Error{"NO_DATA", "No local dataset. Use --dataset FILE or run update --source HTTPS_MANIFEST / --bundle FILE", 3}
	}
	if e != nil {
		return "", nil, e
	}
	var c Current
	if e = Decode(b, &c); e != nil {
		return "", nil, e
	}
	if e = validateManifest(c.Manifest); e != nil {
		return "", nil, e
	}
	return filepath.Join(cache, c.Manifest.SHA256+".json.gz"), &c, nil
}
func validateManifest(m Manifest) error {
	if m.Schema != ManifestSchema || m.BundleSchema != BundleSchema || !regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`).MatchString(m.Snapshot) || !validHash.MatchString(m.SHA256) || !validHash.MatchString(m.CatalogSHA256) || m.Size <= 0 || m.Size > MaxData || m.Works <= 0 || m.AuthorVersion == "" {
		return fmt.Errorf("invalid update schema/version/hash/size/count")
	}
	if m.Release != nil {
		return m.Release.Validate(m)
	}
	return nil
}
func HTTPS(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("source/data URL must be HTTPS without credentials or fragment")
	}
	return u, nil
}
func HTTPClient() *http.Client {
	return &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("too many redirects")
		}
		_, e := HTTPS(r.URL.String())
		return e
	}}
}
func fetch(ctx context.Context, client *http.Client, u string, max int64) ([]byte, *url.URL, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return nil, nil, e
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "openbibliographies-ob/"+ReaderVersion)
	r, e := client.Do(req)
	if e != nil {
		return nil, nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, nil, fmt.Errorf("HTTP %d", r.StatusCode)
	}
	if r.ContentLength > max {
		return nil, nil, fmt.Errorf("download exceeds limit")
	}
	if r.Request == nil {
		return nil, nil, fmt.Errorf("missing final response URL")
	}
	final, e := HTTPS(r.Request.URL.String())
	if e != nil {
		return nil, nil, e
	}
	b, e := ReadBounded(r.Body, max)
	return b, final, e
}
func Update(ctx context.Context, cache, source, bundlePath string, client *http.Client) (Current, error) {
	var zero Current
	if e := ctx.Err(); e != nil {
		return zero, e
	}
	if (source == "") == (bundlePath == "") {
		return zero, Invalid("Use exactly one of --source HTTPS_MANIFEST or --bundle FILE; no production latest URL is configured")
	}
	if source != "" {
		if _, e := HTTPS(source); e != nil {
			return zero, e
		}
	}
	if e := os.MkdirAll(cache, 0700); e != nil {
		return zero, e
	}
	lock := filepath.Join(cache, "update.lock")
	if e := os.Mkdir(lock, 0700); e != nil {
		if errors.Is(e, os.ErrExist) {
			return zero, &Error{"UPDATE_BUSY", "Another update holds the cache lock; after a crash remove update.lock only when no updater is running", 5}
		}
		return zero, e
	}
	defer os.Remove(lock)
	var m Manifest
	var b []byte
	var e error
	if bundlePath != "" {
		b, e = ReadFile(bundlePath)
		if e != nil {
			return zero, e
		}
	} else {
		if client == nil {
			client = HTTPClient()
		}
		copyClient := *client
		copyClient.CheckRedirect = HTTPClient().CheckRedirect
		if copyClient.Timeout <= 0 || copyClient.Timeout > 45*time.Second {
			copyClient.Timeout = 45 * time.Second
		}
		client = &copyClient
		mb, finalManifestURL, e := fetch(ctx, client, source, 64<<10)
		if e != nil {
			return zero, e
		}
		if e = Decode(mb, &m); e != nil {
			return zero, e
		}
		if e = validateManifest(m); e != nil {
			return zero, e
		}
		if m.Release == nil {
			return zero, fmt.Errorf("HTTPS update requires a complete versioned release inventory")
		}
		base := finalManifestURL
		rel, e := url.Parse(m.URL)
		if e != nil {
			return zero, e
		}
		u := base.ResolveReference(rel)
		if _, e = HTTPS(u.String()); e != nil {
			return zero, e
		}
		b, _, e = fetch(ctx, client, u.String(), m.Size)
		if e != nil {
			return zero, e
		}
		if int64(len(b)) != m.Size || Digest(b) != m.SHA256 {
			return zero, fmt.Errorf("download size/hash mismatch")
		}
	}
	if e = ctx.Err(); e != nil {
		return zero, e
	}
	x, e := LoadBytes(b)
	if e != nil {
		return zero, fmt.Errorf("dataset validation: %w", e)
	}
	if bundlePath != "" {
		m = Manifest{ManifestSchema, x.Bundle.Snapshot, x.Bundle.Development, "", Digest(b), int64(len(b)), BundleSchema, x.Bundle.Retrieval.CatalogSHA256, x.Bundle.Retrieval.AuthorVersion, len(x.Catalog.Works), nil}
	}
	if e = validateManifest(m); e != nil {
		return zero, e
	}
	if x.Bundle.Snapshot != m.Snapshot || x.Bundle.Development != m.Development || x.Bundle.Retrieval.CatalogSHA256 != m.CatalogSHA256 || x.Bundle.Retrieval.AuthorVersion != m.AuthorVersion || len(x.Catalog.Works) != m.Works {
		return zero, fmt.Errorf("manifest/dataset metadata mismatch")
	}
	if m.Release != nil {
		if _, e = m.ValidateBundle(b); e != nil {
			return zero, e
		}
	}
	if e = ctx.Err(); e != nil {
		return zero, e
	}
	c := Current{m, time.Now().UTC().Format(time.RFC3339), source}
	if bundlePath != "" {
		c.Source = "local bundle"
	}
	// Content addressed generations are immutable. Only current.json is the commit point.
	target := filepath.Join(cache, m.SHA256+".json.gz")
	if existing, e := os.ReadFile(target); e == nil {
		h := sha256.Sum256(existing)
		if hex.EncodeToString(h[:]) != m.SHA256 {
			return zero, fmt.Errorf("existing cache generation corrupt")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return zero, e
	} else if e = writeAtomic(ctx, cache, target, b); e != nil {
		return zero, e
	}
	cb, e := json.Marshal(c)
	if e != nil {
		return zero, e
	}
	if e = writeAtomic(ctx, cache, filepath.Join(cache, "current.json"), cb); e != nil {
		return zero, e
	}
	return c, nil
}
func writeAtomic(ctx context.Context, dir, target string, b []byte) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".incoming-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = io.Copy(f, bytes.NewReader(b)); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = os.Rename(name, target); e != nil {
		return e
	}
	return nil
}
