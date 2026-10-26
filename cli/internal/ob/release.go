package ob

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const ReaderVersion = "0.3.0"
const ReaderContract = "openbibliographies.cli.v1"

type Artifact struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
	OS     string `json:"os,omitempty"`
	Arch   string `json:"arch,omitempty"`
}
type ByteIdentity struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type CLIIdentity struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	SourceSHA256  string `json:"sourceSHA256"`
	ReaderVersion string `json:"readerVersion"`
}
type LicenseScope struct {
	Status string `json:"status"`
	SPDX   string `json:"spdx"`
	Scope  string `json:"scope"`
	File   string `json:"file"`
}
type LicenseScopes struct {
	Software LicenseScope `json:"software"`
	Data     LicenseScope `json:"data"`
}

func ReleaseLicenses() LicenseScopes {
	return LicenseScopes{LicenseScope{"licensed", "MIT", "Go CLI code; third-party licenses retained separately", "CLI-LICENSE"}, LicenseScope{"licensed", "CC-BY-4.0", "compiled bibliography and classifications", "DATA-LICENSE"}}
}

type OverlayIdentity struct {
	Schema        string `json:"schema"`
	Snapshot      string `json:"snapshot"`
	CatalogSHA256 string `json:"catalogSHA256"`
	Version       string `json:"version"`
	MappingSHA256 string `json:"mappingSHA256"`
}
type Release struct {
	ID                    string              `json:"id"`
	CLI                   CLIIdentity         `json:"cli"`
	MinimumReaderVersion  string              `json:"minimumReaderVersion"`
	SemanticsSHA256       string              `json:"semanticsSHA256"`
	ReaderContract        string              `json:"readerContract"`
	Generated             string              `json:"generated"`
	ClassificationVersion string              `json:"classificationVersion"`
	CatalogSchema         string              `json:"catalogSchema"`
	CatalogSize           int64               `json:"catalogSize"`
	UncompressedBundle    ByteIdentity        `json:"uncompressedBundle"`
	Overlay               OverlayIdentity     `json:"overlay"`
	Artifacts             map[string]Artifact `json:"artifacts"`
	Licenses              LicenseScopes       `json:"licenses"`
}

func (r Release) Validate(m Manifest) error {
	if numericVersion.MatchString(r.MinimumReaderVersion) && !supportsMinimum(r.MinimumReaderVersion) {
		return fmt.Errorf("this data needs ob >= %s; update the CLI before updating data", r.MinimumReaderVersion)
	}
	if !validID.MatchString(r.ID) || r.ReaderContract != ReaderContract || !supportsMinimum(r.MinimumReaderVersion) || !numericVersion.MatchString(r.CLI.ReaderVersion) || r.SemanticsSHA256 != SemanticsSHA256 || r.CLI.Version == "" || !validHash.MatchString(r.CLI.SourceSHA256) || r.CLI.Commit != "sha256:"+r.CLI.SourceSHA256 || r.Generated == "" || r.ClassificationVersion == "" || r.CatalogSchema != "openbibliographies.preview.v4" || r.CatalogSize <= 0 || r.CatalogSize > MaxData || r.UncompressedBundle.Size <= 0 || r.UncompressedBundle.Size > MaxData || !validHash.MatchString(r.UncompressedBundle.SHA256) {
		return fmt.Errorf("unsupported release identity/reader contract/catalog metadata")
	}
	o := r.Overlay
	if o.Schema != "openbibliographies.author-aggregation.v1" || o.Snapshot != m.Snapshot || o.CatalogSHA256 != m.CatalogSHA256 || o.Version != m.AuthorVersion || !validHash.MatchString(o.MappingSHA256) {
		return fmt.Errorf("release author overlay binding mismatch")
	}
	if len(r.Artifacts) < 2 || len(r.Artifacts) > 40 {
		return fmt.Errorf("invalid release inventory")
	}
	for name, a := range r.Artifacts {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || a.Size <= 0 || a.Size > MaxData || !validHash.MatchString(a.SHA256) || a.URL == "" || a.Kind == "" {
			return fmt.Errorf("invalid release artifact %s", name)
		}
	}
	for name, a := range r.Artifacts {
		if a.Kind == "binary" || a.Kind == "archive" {
			if (a.OS != "darwin" && a.OS != "linux" && a.OS != "windows") || (a.Arch != "amd64" && a.Arch != "arm64") {
				return fmt.Errorf("unknown artifact platform")
			}
			if a.Kind == "binary" {
				want := "ob_" + a.OS + "_" + a.Arch
				if a.OS == "windows" {
					want += ".exe"
				}
				if name != want {
					return fmt.Errorf("binary filename/platform mismatch")
				}
			}
		}
	}
	a, ok := r.Artifacts["catalog.json.gz"]
	if !ok || a.URL != m.URL || a.Size != m.Size || a.SHA256 != m.SHA256 {
		return fmt.Errorf("release bundle inventory mismatch")
	}
	if a, ok = r.Artifacts["author-aggregation.json"]; !ok || a.Kind != "author-overlay" {
		return fmt.Errorf("missing release overlay artifact")
	}
	for _, l := range []LicenseScope{r.Licenses.Software, r.Licenses.Data} {
		if l.Status == "" || l.Scope == "" || l.File == "" {
			return fmt.Errorf("missing license scope/status")
		}
	}
	return nil
}
func (m Manifest) Validate() error { return validateManifest(m) }
func (m Manifest) ValidateBundle(b []byte) (*Index, error) {
	if e := m.Validate(); e != nil {
		return nil, e
	}
	if int64(len(b)) != m.Size || Digest(b) != m.SHA256 {
		return nil, fmt.Errorf("bundle byte identity mismatch")
	}
	x, e := LoadBytes(b)
	if e != nil {
		return nil, e
	}
	if x.Bundle.Snapshot != m.Snapshot || x.Bundle.Development != m.Development || x.Bundle.Retrieval.CatalogSHA256 != m.CatalogSHA256 || x.Bundle.Retrieval.AuthorVersion != m.AuthorVersion || len(x.Catalog.Works) != m.Works {
		return nil, fmt.Errorf("bundle manifest metadata mismatch")
	}
	if m.Release != nil {
		r := m.Release
		var overlay AuthorOverlay
		if e := Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &overlay); e != nil {
			return nil, e
		}
		if r.Overlay.MappingSHA256 != overlay.MappingSHA256 {
			return nil, fmt.Errorf("release/embedded overlay mapping digest mismatch")
		}
		if r.CatalogSize != int64(len(x.Bundle.Catalog)) || r.Generated != x.Catalog.Generated || r.ClassificationVersion != x.Catalog.ClassificationVersion || r.Overlay.Version != x.Bundle.Retrieval.AuthorVersion || r.Artifacts["author-aggregation.json"].SHA256 != x.Bundle.Retrieval.OverlaySHA256 {
			return nil, fmt.Errorf("release/catalog/overlay metadata mismatch")
		}
		raw, e := Inflate(b)
		if e != nil {
			return nil, e
		}
		if r.UncompressedBundle.Size != int64(len(raw)) || r.UncompressedBundle.SHA256 != Digest(raw) {
			return nil, fmt.Errorf("uncompressed bundle identity mismatch")
		}
	}
	return x, nil
}

var numericVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func supportsMinimum(v string) bool {
	if !numericVersion.MatchString(v) {
		return false
	}
	want, have := strings.Split(v, "."), strings.Split(ReaderVersion, ".")
	for i := 0; i < 3; i++ {
		a, e := strconv.ParseUint(want[i], 10, 31)
		if e != nil {
			return false
		}
		b, _ := strconv.ParseUint(have[i], 10, 31)
		if a < b {
			return true
		}
		if a > b {
			return false
		}
	}
	return true
}
