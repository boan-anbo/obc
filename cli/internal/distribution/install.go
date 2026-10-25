package distribution

import (
	"errors"
	"fmt"
	"openbibliographies/ob/internal/ob"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

type Installation struct {
	Directory string   `json:"directory"`
	Binary    string   `json:"binary"`
	Dataset   string   `json:"dataset"`
	ReleaseID string   `json:"releaseID"`
	Snapshot  string   `json:"snapshot"`
	Example   []string `json:"example"`
}

func Install(packageDir, destination, runningVersion, runningCommit string) (result Installation, err error) {
	if packageDir == "" || destination == "" {
		return result, ob.Invalid("install requires --package DIR and --dir NEW_DIR")
	}
	p, e := openForInstall(packageDir)
	if e != nil {
		return result, e
	}
	if p.Manifest.Release.CLI.Version != runningVersion || p.Manifest.Release.CLI.Commit != runningCommit || p.Manifest.Release.CLI.ReaderVersion != ob.ReaderVersion {
		return result, fmt.Errorf("running installer/release identity mismatch; use this package's bootstrap")
	}
	name := "ob_" + runtime.GOOS + "_" + runtime.GOARCH
	exe := "ob"
	if runtime.GOOS == "windows" {
		name += ".exe"
		exe += ".exe"
	}
	a, ok := p.Manifest.Release.Artifacts[name]
	if !ok || a.OS != runtime.GOOS || a.Arch != runtime.GOARCH || a.Kind != "binary" {
		return result, fmt.Errorf("package does not support this platform")
	}
	if _, e = InspectBinary(filepath.Join(p.Dir, name), runtime.GOOS, runtime.GOARCH); e != nil {
		return result, e
	}
	if e = p.checkIdentity(name); e != nil {
		return result, e
	}
	data, e := p.Read("catalog.json.gz")
	if e != nil {
		return result, e
	}
	if _, e = p.Manifest.ValidateBundle(data); e != nil {
		return result, e
	}
	binary, e := p.Read(name)
	if e != nil {
		return result, e
	}
	files := map[string][]byte{"catalog.json.gz": data}
	for _, n := range []string{"CLI-README.md", "AGENT_USAGE.md", "CLI-LICENSE", "DATA-LICENSE", "THIRD-PARTY-NOTICES"} {
		b, e := p.Read(n)
		if e != nil {
			return result, e
		}
		files[n] = b
	}
	files["release-manifest.json"] = p.ManifestBytes
	destination, e = filepath.Abs(destination)
	if e != nil {
		return result, e
	}
	if e = os.Mkdir(destination, 0755); e != nil {
		return result, fmt.Errorf("destination must be new with an existing parent: %w", e)
	}
	created := []string{}
	success := false
	defer func() {
		if !success {
			for _, n := range created {
				if e := os.Remove(n); e != nil && !errors.Is(e, os.ErrNotExist) {
					err = errors.Join(err, e)
				}
			}
			if e := os.Remove(destination); e != nil {
				err = errors.Join(err, fmt.Errorf("partial install retained at %s: %w", destination, e))
			}
		}
	}()
	write := func(name string, b []byte, mode os.FileMode) error {
		path := filepath.Join(destination, name)
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if e != nil {
			return e
		}
		created = append(created, path)
		if _, e = f.Write(b); e != nil {
			f.Close()
			return e
		}
		if e = f.Sync(); e != nil {
			f.Close()
			return e
		}
		return f.Close()
	}
	sums := ""
	names := []string{}
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b := files[n]
		if e = write(n, b, 0644); e != nil {
			return result, e
		}
		sums += fmt.Sprintf("%s  %s\n", ob.Digest(b), n)
	}
	sums += fmt.Sprintf("%s  %s\n", ob.Digest(binary), exe)
	if e = write("CLI-SHA256SUMS", []byte(sums), 0644); e != nil {
		return result, e
	}
	if e = write(exe, binary, 0755); e != nil {
		return result, e
	}
	success = true
	path := filepath.Join(destination, exe)
	dataset := filepath.Join(destination, "catalog.json.gz")
	return Installation{destination, path, dataset, p.Manifest.Release.ID, p.Manifest.Snapshot, []string{path, "query", "--dataset", dataset, "--q", "Adams Bodomo"}}, nil
}
