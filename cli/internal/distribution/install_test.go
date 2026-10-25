package distribution

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"openbibliographies/ob/internal/ob"
)

func releasePackage(t *testing.T) *Package {
	t.Helper()
	dir := os.Getenv("OB_RELEASE_PACKAGE")
	if dir == "" {
		t.Skip("OB_RELEASE_PACKAGE required for actual artifact integration")
	}
	p, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func nativeName() string {
	v := "ob_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		v += ".exe"
	}
	return v
}
func minimalPackage(t *testing.T, p *Package) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "package space $literal")
	os.Mkdir(dir, 0755)
	for _, n := range []string{nativeName(), "catalog.json.gz", "CLI-README.md", "AGENT_USAGE.md", "CLI-LICENSE", "DATA-LICENSE", "THIRD-PARTY-NOTICES", "CLI-SHA256SUMS", "install.sh"} {
		b, e := os.ReadFile(filepath.Join(p.Dir, n))
		if e != nil {
			t.Fatal(e)
		}
		mode := os.FileMode(0644)
		if n == nativeName() || n == "install.sh" {
			mode = 0755
		}
		if e = os.WriteFile(filepath.Join(dir, n), b, mode); e != nil {
			t.Fatal(e)
		}
	}
	os.WriteFile(filepath.Join(dir, "cli-manifest.json"), p.ManifestBytes, 0644)
	return dir
}
func TestInstallActualOfflinePackage(t *testing.T) {
	p := releasePackage(t)
	subset := minimalPackage(t, p)
	dest := filepath.Join(t.TempDir(), "安装 with spaces $literal")
	result, e := Install(subset, dest, p.Manifest.Release.CLI.Version, p.Manifest.Release.CLI.Commit)
	if e != nil {
		t.Fatal(e)
	}
	b, e := exec.Command(result.Binary, "query", "--dataset", result.Dataset, "--q", "ΔΗΜΟΣ").Output()
	if e != nil {
		t.Fatal(e)
	}
	var r ob.Result
	json.Unmarshal(b, &r)
	if r.Matched != 1 {
		t.Fatal("installed query failed", string(b))
	}
	sums, e := os.ReadFile(filepath.Join(dest, "CLI-SHA256SUMS"))
	if e != nil {
		t.Fatal(e)
	}
	entries, e := ReadChecksums(sums)
	if e != nil {
		t.Fatal(e)
	}
	for n, h := range entries {
		v, e := os.ReadFile(filepath.Join(dest, n))
		if e != nil || ob.Digest(v) != h {
			t.Fatal("installed checksum mismatch", n, e)
		}
	}
	marker := filepath.Join(dest, "keep")
	os.WriteFile(marker, []byte("owned"), 0600)
	if _, e = Install(subset, dest, p.Manifest.Release.CLI.Version, p.Manifest.Release.CLI.Commit); e == nil {
		t.Fatal("existing directory overwritten")
	}
	if b, _ := os.ReadFile(marker); string(b) != "owned" {
		t.Fatal("existing content changed")
	}
}
func TestInstallerRejectsCorruptMissingAndSymlinkData(t *testing.T) {
	p := releasePackage(t)
	for _, mode := range []string{"hash", "missing", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			pkg := minimalPackage(t, p)
			file := filepath.Join(pkg, "catalog.json.gz")
			switch mode {
			case "hash":
				f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
				f.Write([]byte("bad"))
				f.Close()
			case "missing":
				os.Remove(file)
			case "symlink":
				os.Remove(file)
				if e := os.Symlink(filepath.Join(p.Dir, "catalog.json.gz"), file); e != nil {
					t.Skip(e)
				}
			}
			dest := filepath.Join(t.TempDir(), "absent")
			if _, e := Install(pkg, dest, p.Manifest.Release.CLI.Version, p.Manifest.Release.CLI.Commit); e == nil {
				t.Fatal("invalid package accepted")
			}
			if _, e := os.Lstat(dest); !os.IsNotExist(e) {
				t.Fatal("failure created destination")
			}
		})
	}
}
func TestBootstrapPlatformChecksumAndPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	p := releasePackage(t)
	for _, mode := range []string{"success", "non-executable-binary", "corrupt-binary", "duplicate-checksum", "missing-checksum", "unsupported-os", "unsupported-arch", "missing-argument", "repeated-argument"} {
		t.Run(mode, func(t *testing.T) {
			pkg := minimalPackage(t, p)
			dest := filepath.Join(t.TempDir(), "installed $literal space")
			args := []string{filepath.Join(pkg, "install.sh"), "--package", pkg, "--dir", dest}
			env := append(os.Environ(), "PATH=/usr/bin:/bin")
			switch mode {
			case "non-executable-binary":
				os.Chmod(filepath.Join(pkg, nativeName()), 0644)
			case "corrupt-binary":
				f, _ := os.OpenFile(filepath.Join(pkg, nativeName()), os.O_APPEND|os.O_WRONLY, 0600)
				f.Write([]byte("bad"))
				f.Close()
			case "duplicate-checksum":
				b, _ := os.ReadFile(filepath.Join(pkg, "CLI-SHA256SUMS"))
				for _, line := range strings.Split(string(b), "\n") {
					if strings.HasSuffix(line, "  "+nativeName()) {
						b = append(b, []byte(line+"\n")...)
						break
					}
				}
				os.WriteFile(filepath.Join(pkg, "CLI-SHA256SUMS"), b, 0644)
			case "missing-checksum":
				os.Remove(filepath.Join(pkg, "CLI-SHA256SUMS"))
			case "unsupported-os", "unsupported-arch":
				fake := t.TempDir()
				system, arch := "Darwin", "arm64"
				if runtime.GOOS == "linux" {
					system = "Linux"
				}
				if mode == "unsupported-os" {
					system = "Plan9"
				} else {
					arch = "riscv64"
				}
				os.WriteFile(filepath.Join(fake, "uname"), []byte("#!/bin/sh\ncase \"$1\" in -s) echo "+system+" ;; *) echo "+arch+" ;; esac\n"), 0755)
				env = append(os.Environ(), "PATH="+fake+":/usr/bin:/bin")
			case "missing-argument":
				args = []string{filepath.Join(pkg, "install.sh"), "--dir"}
			case "repeated-argument":
				args = append(args, "--dir", dest)
			}
			cmd := exec.Command("sh", args...)
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			e := cmd.Run()
			if mode == "success" || mode == "non-executable-binary" {
				if e != nil {
					t.Fatal(e, stderr.String())
				}
				var info Installation
				if e = json.Unmarshal(stdout.Bytes(), &info); e != nil {
					t.Fatal(e)
				}
				if b, e := exec.Command(info.Binary, "query", "--dataset", info.Dataset, "--q", "Adams Bodomo").Output(); e != nil || !bytes.Contains(b, []byte(`"matched":5`)) {
					t.Fatal(e, string(b))
				}
			} else {
				if e == nil {
					t.Fatal("bad install succeeded")
				}
				if _, e = os.Lstat(dest); !os.IsNotExist(e) {
					t.Fatal("invalid bootstrap installed content")
				}
				if stdout.Len() != 0 {
					t.Fatal("error wrote stdout", stdout.String())
				}
			}
		})
	}
}
func TestInspectAllActualTargets(t *testing.T) {
	p := releasePackage(t)
	r, e := p.Inspect()
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Binaries) != 6 || r.Archives != 6 {
		t.Fatal(r)
	}
	for _, v := range r.Binaries {
		if v.NativeExecuted {
			t.Fatal("inspection claimed native execution")
		}
	}
	other := "amd64"
	if runtime.GOARCH == other {
		other = "arm64"
	}
	if _, e = InspectBinary(filepath.Join(p.Dir, nativeName()), runtime.GOOS, other); e == nil {
		t.Fatal("wrong architecture accepted")
	}
}
func TestChecksumNamesAndDuplicates(t *testing.T) {
	h := strings.Repeat("a", 64)
	for _, s := range []string{h + "  ../x\n" + h + "  y", h + "  x\n" + h + "  x", "bad  x"} {
		if _, e := ReadChecksums([]byte(s)); e == nil {
			t.Fatal("unsafe checksums accepted")
		}
	}
}

func TestBootstrapMissingPackageExitsFourWithoutInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap")
	}
	root := t.TempDir()
	destination := filepath.Join(root, "not-installed")
	cmd := exec.Command("sh", "../../scripts/install-local.sh", "--package", filepath.Join(root, "missing-package"), "--dir", destination)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 4 {
		t.Fatalf("expected exit 4; got %v: %s", err, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatal("invalid diagnostic streams")
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("failed bootstrap created destination")
	}
}
