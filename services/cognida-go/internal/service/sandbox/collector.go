package sandbox

import (
	"archive/tar"
	"bytes"
	d "cognida/internal/model/sandbox"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Legacy TAR archives use NUL for a regular file; retain that accepted wire value.
const legacyTarRegularFile byte = 0

type CollectedFile struct {
	Artifact d.Artifact
	Body     []byte
}

func CollectArchive(v ValidatedSubmission, archive []byte) (d.Manifest, []CollectedFile, error) {
	manifest := d.Manifest{SchemaVersion: d.SchemaVersion, ExecutionID: v.Binding.ID, RequestSHA256: v.Binding.RequestSHA256, Disposition: d.CandidateOnly, Artifacts: []d.Artifact{}}
	files := []CollectedFile{}
	tr := tar.NewReader(bytes.NewReader(archive))
	seen := map[string]bool{}
	var size int64
	entries := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return manifest, nil, d.ErrInvalid
		}
		entries++
		if entries > int(v.Request.Resources.OutputFiles)+128 {
			return manifest, nil, d.ErrInvalid
		}
		name := strings.TrimPrefix(h.Name, "./")
		if h.Typeflag == tar.TypeDir {
			if name != "." && name != "" && !safeOutputPath(strings.TrimSuffix(name, "/")) {
				return manifest, nil, d.ErrInvalid
			}
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != legacyTarRegularFile {
			return manifest, nil, invalid("output links, archives and special files are forbidden")
		}
		if name == ".cognida-result.json" || name == ".cognida-log" {
			bound := v.Request.Resources.LogBytes
			if name == ".cognida-result.json" {
				bound = 2048
			}
			if h.Size < 0 || h.Size > bound {
				return manifest, nil, d.ErrInvalid
			}
			continue
		}
		if !safeOutputPath(name) || seen[name] || h.Size < 0 || h.Size > v.Request.Resources.OutputBytes-size {
			return manifest, nil, d.ErrInvalid
		}
		seen[name] = true
		if int64(len(files)) >= v.Request.Resources.OutputFiles {
			return manifest, nil, d.ErrInvalid
		}
		body, e := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if e != nil || int64(len(body)) != h.Size || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
			return manifest, nil, d.ErrInvalid
		}
		media := "text/plain"
		switch strings.ToLower(path.Ext(name)) {
		case ".json":
			if !json.Valid(body) {
				return manifest, nil, d.ErrInvalid
			}
			media = "application/json"
		case ".txt", ".md":
		default:
			return manifest, nil, invalid("only JSON and UTF-8 text outputs supported")
		}
		digest := sha256.Sum256(body)
		a := d.Artifact{Path: name, Kind: d.RegularFile, MediaType: media, SizeBytes: h.Size, SHA256: "sha256:" + hex.EncodeToString(digest[:])}
		manifest.Artifacts = append(manifest.Artifacts, a)
		files = append(files, CollectedFile{a, body})
		size += h.Size
	}
	if e := ValidateManifestMetadata(v.BindingPrincipal(), v, manifest); e != nil {
		return manifest, nil, e
	}
	return manifest, files, nil
}
