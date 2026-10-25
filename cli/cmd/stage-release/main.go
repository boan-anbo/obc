// Prepare a new local static tree. Does not write an existing site or publish.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"openbibliographies/ob/internal/distribution"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	pkg := flag.String("package", "dist/release", "verified package directory")
	out := flag.String("out", "dist/static-release", "new staging root")
	flag.Parse()
	if e := run(*pkg, *out); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(pkg, out string) error {
	p, e := distribution.Open(pkg)
	if e != nil {
		return e
	}
	if _, e = p.Inspect(); e != nil {
		return e
	}
	data, e := p.Read("catalog.json.gz")
	if e != nil {
		return e
	}
	if _, e = p.Manifest.ValidateBundle(data); e != nil {
		return e
	}
	u, e := url.Parse(p.Manifest.URL)
	if e != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || path.Clean(u.Path) != u.Path {
		return fmt.Errorf("staging requires root-relative static paths")
	}
	releasePath := path.Dir(u.Path)
	base, rest, ok := strings.Cut(releasePath, "/releases/")
	if !ok || rest != p.Manifest.Release.ID {
		return fmt.Errorf("static path/release ID mismatch")
	}
	if e = os.Mkdir(out, 0755); e != nil {
		return e
	}
	dest := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(releasePath, "/")))
	if e = os.MkdirAll(dest, 0755); e != nil {
		return e
	}
	for name, a := range p.Manifest.Release.Artifacts {
		b, e := p.Read(name)
		if e != nil {
			return e
		}
		mode := os.FileMode(0644)
		if a.Kind == "binary" || name == "install.sh" {
			mode = 0755
		}
		if e = os.WriteFile(filepath.Join(dest, name), b, mode); e != nil {
			return e
		}
	}
	if e = os.WriteFile(filepath.Join(dest, "cli-manifest.json"), p.ManifestBytes, 0644); e != nil {
		return e
	}
	if _, e = distribution.Open(dest); e != nil {
		return e
	}
	latest := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(base, "/")), "latest.json")
	temp := latest + ".pending"
	if e = os.WriteFile(temp, p.ManifestBytes, 0644); e != nil {
		return e
	}
	if e = os.Rename(temp, latest); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"releaseID": p.Manifest.Release.ID, "immutableDirectory": dest, "latest": latest, "publication": "not performed"})
}
