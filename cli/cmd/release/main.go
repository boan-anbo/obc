// Build deterministic, local-only release releases. This command never publishes.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"openbibliographies/ob/internal/ob"
)

func main() {
	out := flag.String("out", "dist/release", "new output directory")
	dataset := flag.String("dataset", "dist/catalog.json.gz", "validated bundle")
	version := flag.String("version", "0.3.0", "release version")
	publicBase := flag.String("public-base", "/cli", "static URL path prefix; no hostname is invented")
	assetBase := flag.String("asset-base", "", "optional immutable HTTPS directory for flat release assets")
	flag.Parse()
	if e := run(*out, *dataset, *version, *publicBase, *assetBase); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(out, dataPath, version, publicBase, assetBase string) error {
	if !strings.HasPrefix(publicBase, "/") || path.Clean(publicBase) != publicBase || strings.ContainsAny(publicBase, "?#\\") {
		return fmt.Errorf("invalid static path prefix")
	}
	if strings.ContainsAny(version, " \t\r\n'\"") {
		return fmt.Errorf("invalid version")
	}
	if assetBase != "" {
		u, err := url.Parse(assetBase)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid HTTPS asset base")
		}
	}
	data, e := ob.ReadFile(dataPath)
	if e != nil {
		return e
	}
	x, e := ob.LoadBytes(data)
	if e != nil {
		return e
	}
	if e = os.Mkdir(out, 0755); e != nil {
		return e
	}
	sourceHash := sha256.New()
	files := []string{"go.mod", "go.sum"}
	for _, dir := range []string{"cmd", "internal"} {
		e = filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") {
				files = append(files, p)
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	sort.Strings(files)
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		fmt.Fprintf(sourceHash, "%s\x00", filepath.ToSlash(p))
		sourceHash.Write(b)
	}
	identity := hex.EncodeToString(sourceHash.Sum(nil))
	payload := map[string][]byte{}
	for _, p := range []string{"CLI-README.md", "AGENT_USAGE.md", "CLI-LICENSE", "THIRD-PARTY-NOTICES", "DATA-LICENSE", "UPDATE-CONTRACT.md"} {
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		payload[p] = b
	}
	for name, b := range payload {
		if e = os.WriteFile(filepath.Join(out, name), b, 0644); e != nil {
			return e
		}
	}
	for src, dst := range map[string]string{"scripts/install-local.sh": "install.sh", "scripts/install-local.ps1": "install.ps1"} {
		b, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(out, dst), b, 0755); e != nil {
			return e
		}
	}
	artifacts := map[string]ob.Artifact{}
	for _, target := range [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		osname, arch := target[0], target[1]
		name := "ob_" + osname + "_" + arch
		exe := "ob"
		if osname == "windows" {
			name += ".exe"
			exe += ".exe"
		}
		path := filepath.Join(out, name)
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -buildid= -X main.version="+version+" -X main.commit=sha256:"+identity, "-o", path, "./cmd/ob")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+osname, "GOARCH="+arch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e = cmd.Run(); e != nil {
			return e
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		entries := map[string][]byte{}
		for p, b := range payload {
			entries[p] = b
		}
		entries[exe] = b
		archive := filepath.Join(out, "ob-"+version+"-"+osname+"-"+arch)
		if osname == "windows" {
			e = zipArchive(archive+".zip", entries, exe)
		} else {
			e = tarArchive(archive+".tar.gz", entries, exe)
		}
		if e != nil {
			return e
		}
		artifacts[name] = ob.Artifact{Kind: "binary", OS: osname, Arch: arch}
		archiveName := filepath.Base(archive) + ".tar.gz"
		if osname == "windows" {
			archiveName = filepath.Base(archive) + ".zip"
		}
		artifacts[archiveName] = ob.Artifact{Kind: "archive", OS: osname, Arch: arch}
		fmt.Printf("%s %d bytes\n", name, len(b))
	}
	if e = os.WriteFile(filepath.Join(out, "catalog.json.gz"), data, 0644); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(out, "author-aggregation.json"), []byte(x.Bundle.Retrieval.OverlayJSON), 0644); e != nil {
		return e
	}
	provenance := map[string]string{"version": version, "sourceSHA256": identity, "goVersion": strings.TrimSpace(goVersion()), "semanticsSHA256": ob.SemanticsSHA256, "catalogSHA256": x.Bundle.Retrieval.CatalogSHA256, "authorOverlaySHA256": x.Bundle.Retrieval.OverlaySHA256}
	pb, _ := json.MarshalIndent(provenance, "", "  ")
	if e = os.WriteFile(filepath.Join(out, "build.json"), append(pb, '\n'), 0644); e != nil {
		return e
	}
	releaseID := strings.ReplaceAll(strings.ToLower(version), ".", "-") + "-" + identity[:12]
	prefix := publicBase + "/releases/" + releaseID + "/"
	if assetBase != "" {
		prefix = strings.TrimRight(assetBase, "/") + "/"
	}
	entries, e := os.ReadDir(out)
	if e != nil {
		return e
	}
	var sums strings.Builder
	for _, f := range entries {
		b, e := os.ReadFile(filepath.Join(out, f.Name()))
		if e != nil {
			return e
		}
		a := artifacts[f.Name()]
		if a.Kind == "" {
			a.Kind = "documentation"
			switch f.Name() {
			case "catalog.json.gz":
				a.Kind = "bundle"
			case "author-aggregation.json":
				a.Kind = "author-overlay"
			case "install.sh", "install.ps1":
				a.Kind = "installer"
			case "build.json":
				a.Kind = "build-metadata"
			}
		}
		a.URL = prefix + f.Name()
		a.Size = int64(len(b))
		a.SHA256 = ob.Digest(b)
		artifacts[f.Name()] = a
		fmt.Fprintf(&sums, "%s  %s\n", a.SHA256, f.Name())
	}
	sumBytes := []byte(sums.String())
	if e = os.WriteFile(filepath.Join(out, "CLI-SHA256SUMS"), sumBytes, 0644); e != nil {
		return e
	}
	artifacts["CLI-SHA256SUMS"] = ob.Artifact{URL: prefix + "CLI-SHA256SUMS", Size: int64(len(sumBytes)), SHA256: ob.Digest(sumBytes), Kind: "checksums"}
	raw, e := ob.Inflate(data)
	if e != nil {
		return e
	}
	var overlay ob.AuthorOverlay
	if e = ob.Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &overlay); e != nil {
		return e
	}
	release := &ob.Release{ID: releaseID, CLI: ob.CLIIdentity{Version: version, Commit: "sha256:" + identity, SourceSHA256: identity, ReaderVersion: ob.ReaderVersion}, MinimumReaderVersion: ob.ReaderVersion, ReaderContract: ob.ReaderContract, SemanticsSHA256: ob.SemanticsSHA256, Generated: x.Catalog.Generated, ClassificationVersion: x.Catalog.ClassificationVersion, CatalogSchema: x.Catalog.Schema, CatalogSize: int64(len(x.Bundle.Catalog)), UncompressedBundle: ob.ByteIdentity{Size: int64(len(raw)), SHA256: ob.Digest(raw)}, Overlay: ob.OverlayIdentity{Schema: overlay.Schema, Snapshot: overlay.Snapshot, CatalogSHA256: overlay.CatalogSHA256, Version: overlay.Version, MappingSHA256: overlay.MappingSHA256}, Artifacts: artifacts, Licenses: ob.ReleaseLicenses()}
	m := ob.Manifest{Schema: ob.ManifestSchema, Snapshot: x.Bundle.Snapshot, Development: x.Bundle.Development, URL: prefix + "catalog.json.gz", SHA256: ob.Digest(data), Size: int64(len(data)), BundleSchema: ob.BundleSchema, CatalogSHA256: x.Bundle.Retrieval.CatalogSHA256, AuthorVersion: x.Bundle.Retrieval.AuthorVersion, Works: len(x.Catalog.Works), Release: release}
	if _, e = m.ValidateBundle(data); e != nil {
		return e
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(out, "cli-manifest.json"), append(mb, '\n'), 0644)
}

func goVersion() string { b, _ := exec.Command("go", "version").Output(); return string(b) }

var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func names(entries map[string][]byte) []string {
	keys := []string{}
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func tarArchive(path string, entries map[string][]byte, exe string) error {
	var b bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	for _, k := range names(entries) {
		mode := int64(0644)
		if k == exe {
			mode = 0755
		}
		if e := tw.WriteHeader(&tar.Header{Name: k, Mode: mode, Size: int64(len(entries[k])), ModTime: epoch, Format: tar.FormatUSTAR}); e != nil {
			return e
		}
		if _, e := tw.Write(entries[k]); e != nil {
			return e
		}
	}
	if e := tw.Close(); e != nil {
		return e
	}
	if e := gz.Close(); e != nil {
		return e
	}
	return os.WriteFile(path, b.Bytes(), 0644)
}
func zipArchive(path string, entries map[string][]byte, exe string) error {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, k := range names(entries) {
		h := &zip.FileHeader{Name: k, Method: zip.Deflate}
		h.SetModTime(epoch)
		mode := fs.FileMode(0644)
		if k == exe {
			mode = 0755
		}
		h.SetMode(mode)
		w, e := zw.CreateHeader(h)
		if e != nil {
			return e
		}
		if _, e = io.Copy(w, bytes.NewReader(entries[k])); e != nil {
			return e
		}
	}
	if e := zw.Close(); e != nil {
		return e
	}
	return os.WriteFile(path, b.Bytes(), 0644)
}
