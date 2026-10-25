// Package distribution verifies release files without executing foreign binaries.
package distribution

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"openbibliographies/ob/internal/ob"
)

type BinaryReport struct {
	File           string `json:"file"`
	Format         string `json:"format"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	CGO            string `json:"cgo"`
	NativeExecuted bool   `json:"nativeExecuted"`
}
type Report struct {
	ReleaseID string         `json:"releaseID"`
	Files     int            `json:"verifiedFiles"`
	Binaries  []BinaryReport `json:"binaries"`
	Archives  int            `json:"verifiedArchives"`
}
type Package struct {
	Dir           string
	ManifestBytes []byte
	Manifest      ob.Manifest
	Checksums     map[string]string
	Report        Report
}

func regular(path string) error {
	s, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !s.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	return nil
}
func ReadChecksums(b []byte) (map[string]string, error) {
	if len(b) > 64<<10 {
		return nil, fmt.Errorf("checksums too large")
	}
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		p := strings.Fields(line)
		if len(p) != 2 || len(p[0]) != 64 || strings.ContainsAny(p[1], "/\\") || p[1] == "." || p[1] == ".." {
			return nil, fmt.Errorf("invalid checksum line")
		}
		if _, e := hex.DecodeString(p[0]); e != nil {
			return nil, e
		}
		if _, ok := m[p[1]]; ok {
			return nil, fmt.Errorf("duplicate checksum")
		}
		m[p[1]] = p[0]
	}
	if len(m) < 2 || len(m) > 40 {
		return nil, fmt.Errorf("invalid checksum count")
	}
	return m, nil
}
func fileHash(path string) (string, int64, error) {
	if e := regular(path); e != nil {
		return "", 0, e
	}
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, ob.MaxData+1))
	if e != nil {
		return "", 0, e
	}
	if n > ob.MaxData {
		return "", 0, fmt.Errorf("artifact exceeds limit")
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
func Open(dir string) (*Package, error)           { return openPackage(dir, true) }
func openForInstall(dir string) (*Package, error) { return openPackage(dir, false) }
func openPackage(dir string, all bool) (*Package, error) {
	dir, e := filepath.Abs(dir)
	if e != nil {
		return nil, e
	}
	for _, name := range []string{"cli-manifest.json", "CLI-SHA256SUMS"} {
		if e = regular(filepath.Join(dir, name)); e != nil {
			return nil, e
		}
	}
	b, e := ob.ReadFile(filepath.Join(dir, "cli-manifest.json"))
	if e != nil {
		return nil, e
	}
	if len(b) > 64<<10 {
		return nil, fmt.Errorf("manifest too large")
	}
	var m ob.Manifest
	if e = ob.Decode(b, &m); e != nil {
		return nil, e
	}
	if e = m.Validate(); e != nil {
		return nil, e
	}
	if m.Release == nil {
		return nil, fmt.Errorf("complete release inventory required")
	}
	sumBytes, e := ob.ReadFile(filepath.Join(dir, "CLI-SHA256SUMS"))
	if e != nil {
		return nil, e
	}
	sums, e := ReadChecksums(sumBytes)
	if e != nil {
		return nil, e
	}
	sa, ok := m.Release.Artifacts["CLI-SHA256SUMS"]
	if !ok || sa.Size != int64(len(sumBytes)) || sa.SHA256 != ob.Digest(sumBytes) {
		return nil, fmt.Errorf("checksums/manifest binding mismatch")
	}
	p := &Package{Dir: dir, ManifestBytes: b, Manifest: m, Checksums: sums, Report: Report{ReleaseID: m.Release.ID, Binaries: []BinaryReport{}}}
	var total int64
	for name, sha := range sums {
		a, ok := m.Release.Artifacts[name]
		if !ok || a.SHA256 != sha {
			return nil, fmt.Errorf("inventory/checksum mismatch: %s", name)
		}
		hash, n, e := fileHash(filepath.Join(dir, name))
		if e != nil {
			if !all && os.IsNotExist(e) {
				continue
			}
			return nil, e
		}
		total += n
		if total > 256<<20 {
			return nil, fmt.Errorf("package exceeds total limit")
		}
		if hash != sha || a.Size != n {
			return nil, fmt.Errorf("artifact hash/size mismatch: %s", name)
		}
		p.Report.Files++
	}
	for name := range m.Release.Artifacts {
		if name != "CLI-SHA256SUMS" {
			if _, ok := sums[name]; !ok {
				return nil, fmt.Errorf("artifact omitted from checksums: %s", name)
			}
		}
	}
	return p, nil
}
func (p *Package) Read(name string) ([]byte, error) {
	a, ok := p.Manifest.Release.Artifacts[name]
	if !ok {
		return nil, fmt.Errorf("missing artifact %s", name)
	}
	if e := regular(filepath.Join(p.Dir, name)); e != nil {
		return nil, e
	}
	b, e := ob.ReadFile(filepath.Join(p.Dir, name))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) != a.Size || ob.Digest(b) != a.SHA256 {
		return nil, fmt.Errorf("artifact changed: %s", name)
	}
	return b, nil
}
func InspectBinary(path, osname, arch string) (BinaryReport, error) {
	r := BinaryReport{File: filepath.Base(path), OS: osname, Arch: arch}
	switch osname {
	case "darwin":
		f, e := macho.Open(path)
		if e != nil {
			return r, e
		}
		defer f.Close()
		want := macho.CpuAmd64
		if arch == "arm64" {
			want = macho.CpuArm64
		}
		if f.Cpu != want {
			return r, fmt.Errorf("Mach-O architecture mismatch")
		}
		r.Format = "Mach-O"
	case "linux":
		f, e := elf.Open(path)
		if e != nil {
			return r, e
		}
		defer f.Close()
		want := elf.EM_X86_64
		if arch == "arm64" {
			want = elf.EM_AARCH64
		}
		if f.Machine != want {
			return r, fmt.Errorf("ELF architecture mismatch")
		}
		for _, ph := range f.Progs {
			if ph.Type == elf.PT_INTERP {
				return r, fmt.Errorf("unexpected dynamic interpreter")
			}
		}
		r.Format = "ELF static"
	case "windows":
		f, e := pe.Open(path)
		if e != nil {
			return r, e
		}
		defer f.Close()
		want := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
		if arch == "arm64" {
			want = pe.IMAGE_FILE_MACHINE_ARM64
		}
		if f.Machine != want {
			return r, fmt.Errorf("PE architecture mismatch")
		}
		r.Format = "PE"
	default:
		return r, fmt.Errorf("unsupported OS")
	}
	info, e := buildinfo.ReadFile(path)
	if e != nil {
		return r, e
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	r.CGO = settings["CGO_ENABLED"]
	if settings["GOOS"] != osname || settings["GOARCH"] != arch || r.CGO != "0" {
		return r, fmt.Errorf("Go platform/CGO metadata mismatch")
	}
	return r, nil
}

// Trimmed Go builds omit linker flags from build info. Inspect embedded identity
// strings under the already verified whole-file hash; this is not native execution.
func (p *Package) checkIdentity(name string) error {
	b, e := p.Read(name)
	if e != nil {
		return e
	}
	id := p.Manifest.Release.CLI
	if !bytes.Contains(b, []byte(id.Version)) || !bytes.Contains(b, []byte(id.Commit)) {
		return fmt.Errorf("embedded binary/release identity mismatch: %s", name)
	}
	return nil
}
func (p *Package) Inspect() (Report, error) {
	for name, a := range p.Manifest.Release.Artifacts {
		if a.Kind == "binary" {
			r, e := InspectBinary(filepath.Join(p.Dir, name), a.OS, a.Arch)
			if e != nil {
				return p.Report, e
			}
			if e = p.checkIdentity(name); e != nil {
				return p.Report, e
			}
			p.Report.Binaries = append(p.Report.Binaries, r)
		}
		if a.Kind == "archive" {
			if e := p.inspectArchive(name, a); e != nil {
				return p.Report, e
			}
			p.Report.Archives++
		}
	}
	if len(p.Report.Binaries) != 6 || p.Report.Archives != 6 {
		return p.Report, fmt.Errorf("expected six binaries and six archives")
	}
	return p.Report, nil
}
func (p *Package) inspectArchive(name string, a ob.Artifact) error {
	b, e := p.Read(name)
	if e != nil {
		return e
	}
	exe := "ob"
	original := "ob_" + a.OS + "_" + a.Arch
	if a.OS == "windows" {
		exe += ".exe"
		original += ".exe"
	}
	seen := map[string]bool{}
	check := func(name string, r io.Reader) error {
		if strings.ContainsAny(name, "/\\") || seen[name] {
			return fmt.Errorf("invalid archive entry")
		}
		seen[name] = true
		key := name
		if name == exe {
			key = original
		}
		expected, ok := p.Manifest.Release.Artifacts[key]
		if !ok {
			return fmt.Errorf("unknown archive entry %s", name)
		}
		v, e := ob.ReadBounded(r, ob.MaxData)
		if e != nil {
			return e
		}
		if int64(len(v)) != expected.Size || ob.Digest(v) != expected.SHA256 {
			return fmt.Errorf("archive entry mismatch %s", name)
		}
		return nil
	}
	if strings.HasSuffix(name, ".zip") {
		z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if e != nil {
			return e
		}
		for _, f := range z.File {
			r, e := f.Open()
			if e != nil {
				return e
			}
			e = check(f.Name, r)
			r.Close()
			if e != nil {
				return e
			}
		}
	} else {
		z, e := gzip.NewReader(bytes.NewReader(b))
		if e != nil {
			return e
		}
		defer z.Close()
		r := tar.NewReader(z)
		for {
			h, e := r.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			if h.Typeflag != tar.TypeReg {
				return fmt.Errorf("unexpected archive entry type")
			}
			if e = check(h.Name, r); e != nil {
				return e
			}
		}
	}
	if !seen[exe] {
		return fmt.Errorf("archive missing executable")
	}
	return nil
}
