package sandbox_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	svc "cognida/internal/service/sandbox"
)

func manifest(s svc.ValidatedSubmission) d.Manifest {
	return d.Manifest{
		SchemaVersion: d.SchemaVersion, ExecutionID: s.Binding.ID, RequestSHA256: s.Binding.RequestSHA256, Disposition: d.CandidateOnly,
		Artifacts: []d.Artifact{{Path: "reports/quality.json", Kind: d.RegularFile, MediaType: "application/json", SizeBytes: 80, SHA256: digest("d")}},
	}
}

func TestManifestMetadataCandidateContract(t *testing.T) {
	s := validated(t)
	m := manifest(s)
	if err := svc.ValidateManifestMetadata(principal, s, m); err != nil {
		t.Fatal(err)
	}
	// This path is only a declaration. Passing metadata checks deliberately does
	// not claim it exists, contains these bytes, or is safe to register as an asset.
	m.Artifacts[0].Path = "not-created/report.json"
	if err := svc.ValidateManifestMetadata(principal, s, m); err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateManifestMetadata(lifecycle.Principal{TenantID: 8, UserID: 11}, s, m); !errors.Is(err, d.ErrForbidden) {
		t.Fatalf("foreign collection: %v", err)
	}
	if err := svc.ValidateManifestMetadata(lifecycle.Principal{TenantID: 7, UserID: 12}, s, m); !errors.Is(err, d.ErrForbidden) {
		t.Fatalf("other user collection: %v", err)
	}
	s.Request.Resources.OutputBytes--
	if err := svc.ValidateManifestMetadata(principal, s, m); !errors.Is(err, d.ErrConflict) {
		t.Fatalf("mutated snapshot accepted: %v", err)
	}
	s = validated(t)
	s.Profile.AllowedMediaTypes = append(s.Profile.AllowedMediaTypes, "application/octet-stream")
	if err := svc.ValidateManifestMetadata(principal, s, m); !errors.Is(err, d.ErrConflict) {
		t.Fatalf("mutated output policy accepted: %v", err)
	}
}

func TestManifestRejectsUntrustedOutputDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		change func(*d.Manifest)
		want   error
	}{
		{"schema", func(m *d.Manifest) { m.SchemaVersion = "2" }, d.ErrConflict},
		{"execution", func(m *d.Manifest) { m.ExecutionID = "someone-elses-execution" }, d.ErrConflict},
		{"contract digest", func(m *d.Manifest) { m.RequestSHA256 = digest("f") }, d.ErrConflict},
		{"publication", func(m *d.Manifest) { m.Disposition = "published" }, d.ErrInvalid},
		{"no artifacts", func(m *d.Manifest) { m.Artifacts = nil }, d.ErrInvalid},
		{"file count", func(m *d.Manifest) { m.Artifacts = make([]d.Artifact, 11) }, d.ErrInvalid},
		{"absolute", func(m *d.Manifest) { m.Artifacts[0].Path = "/etc/passwd" }, d.ErrInvalid},
		{"parent", func(m *d.Manifest) { m.Artifacts[0].Path = "../escape" }, d.ErrInvalid},
		{"nested parent", func(m *d.Manifest) { m.Artifacts[0].Path = "output/../escape" }, d.ErrInvalid},
		{"dot", func(m *d.Manifest) { m.Artifacts[0].Path = "./report.json" }, d.ErrInvalid},
		{"backslash", func(m *d.Manifest) { m.Artifacts[0].Path = `..\escape` }, d.ErrInvalid},
		{"windows drive", func(m *d.Manifest) { m.Artifacts[0].Path = `C:\report.json` }, d.ErrInvalid},
		{"url", func(m *d.Manifest) { m.Artifacts[0].Path = "https://host/report.json" }, d.ErrInvalid},
		{"encoded parent", func(m *d.Manifest) { m.Artifacts[0].Path = "%2e%2e/escape" }, d.ErrInvalid},
		{"empty", func(m *d.Manifest) { m.Artifacts[0].Path = "" }, d.ErrInvalid},
		{"trailing slash", func(m *d.Manifest) { m.Artifacts[0].Path = "reports/" }, d.ErrInvalid},
		{"double slash", func(m *d.Manifest) { m.Artifacts[0].Path = "reports//report.json" }, d.ErrInvalid},
		{"long path", func(m *d.Manifest) { m.Artifacts[0].Path = strings.Repeat("a/", 257) + "b" }, d.ErrInvalid},
		{"control character", func(m *d.Manifest) { m.Artifacts[0].Path = "report\x00.json" }, d.ErrInvalid},
		{"duplicate", func(m *d.Manifest) { m.Artifacts = append(m.Artifacts, m.Artifacts[0]) }, d.ErrInvalid},
		{"file parent before", func(m *d.Manifest) {
			a := m.Artifacts[0]
			a.Path = "reports"
			m.Artifacts = append([]d.Artifact{a}, m.Artifacts...)
		}, d.ErrInvalid},
		{"file parent after", func(m *d.Manifest) { a := m.Artifacts[0]; a.Path = "reports"; m.Artifacts = append(m.Artifacts, a) }, d.ErrInvalid},
		{"symlink", func(m *d.Manifest) { m.Artifacts[0].Kind = "symlink" }, d.ErrInvalid},
		{"hardlink", func(m *d.Manifest) { m.Artifacts[0].Kind = "hardlink" }, d.ErrInvalid},
		{"socket", func(m *d.Manifest) { m.Artifacts[0].Kind = "socket" }, d.ErrInvalid},
		{"directory", func(m *d.Manifest) { m.Artifacts[0].Kind = "directory" }, d.ErrInvalid},
		{"declared link target", func(m *d.Manifest) { m.Artifacts[0].LinkTarget = "../../source" }, d.ErrInvalid},
		{"media", func(m *d.Manifest) { m.Artifacts[0].MediaType = "application/zip" }, d.ErrInvalid},
		{"media spoof declaration", func(m *d.Manifest) { m.Artifacts[0].MediaType = "APPLICATION/JSON" }, d.ErrInvalid},
		{"digest", func(m *d.Manifest) { m.Artifacts[0].SHA256 = "not-a-hash" }, d.ErrInvalid},
		{"negative size", func(m *d.Manifest) { m.Artifacts[0].SizeBytes = -1 }, d.ErrInvalid},
		{"size overflow", func(m *d.Manifest) { m.Artifacts[0].SizeBytes = math.MaxInt64 }, d.ErrInvalid},
		{"aggregate size", func(m *d.Manifest) {
			a := m.Artifacts[0]
			a.Path = "other.json"
			a.SizeBytes = profile().MaxResources.OutputBytes
			m.Artifacts = append(m.Artifacts, a)
		}, d.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validated(t)
			m := manifest(s)
			tt.change(&m)
			if err := svc.ValidateManifestMetadata(principal, s, m); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}
