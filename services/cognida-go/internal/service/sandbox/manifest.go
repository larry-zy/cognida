package sandbox

import (
	"path"
	"strings"

	"cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
)

// ValidateManifestMetadata validates declarations only. It does not read files,
// verify byte digests or MIME, detect disguised links/archives, or authorize
// asset registration. A future collector must securely inspect the real output
// tree and bound archive expansion; this function is not that collector.
func ValidateManifestMetadata(p lifecycle.Principal, submission ValidatedSubmission, manifest d.Manifest) error {
	if err := ValidateExecutionAccess(p, submission.Binding); err != nil {
		return err
	}
	// Detect accidental mutation of the validated snapshot. This hash is not a
	// signature: a future collector must compare against its persisted binding,
	// not accept a caller-supplied snapshot as authorization.
	if submission.Request.ExecutionID != submission.Binding.ID || submission.Request.IdempotencyKey != submission.Binding.IdempotencyKey || submissionDigest(p, submission.Request, submission.Profile) != submission.Binding.RequestSHA256 {
		return d.ErrConflict
	}
	if manifest.SchemaVersion != d.SchemaVersion || manifest.ExecutionID != submission.Binding.ID || manifest.RequestSHA256 != submission.Binding.RequestSHA256 {
		return d.ErrConflict
	}
	if manifest.Disposition != d.CandidateOnly {
		return invalid("output may only be a candidate")
	}
	limits := submission.Request.Resources
	if err := validateLimits(limits); err != nil {
		return err
	}
	if err := validateProfile(submission.Profile); err != nil {
		return err
	}
	if !withinLimits(limits, submission.Profile.MaxResources) {
		return invalid("output contract exceeds profile")
	}
	if len(manifest.Artifacts) == 0 || int64(len(manifest.Artifacts)) > limits.OutputFiles {
		return invalid("invalid output file count")
	}
	mediaTypes := map[string]bool{}
	for _, kind := range submission.Profile.AllowedMediaTypes {
		mediaTypes[kind] = true
	}
	seen := map[string]bool{}
	var size int64
	for _, artifact := range manifest.Artifacts {
		if !safeOutputPath(artifact.Path) || seen[artifact.Path] {
			return invalid("invalid or duplicate output path")
		}
		// A file cannot also be an ancestor directory of another declared file.
		for parent := path.Dir(artifact.Path); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return invalid("conflicting output paths")
			}
		}
		for previous := range seen {
			if strings.HasPrefix(previous, artifact.Path+"/") {
				return invalid("conflicting output paths")
			}
		}
		if artifact.Kind != d.RegularFile || artifact.LinkTarget != "" {
			return invalid("links and special files are forbidden")
		}
		if !mediaTypes[artifact.MediaType] || !digestPattern.MatchString(artifact.SHA256) || artifact.SizeBytes < 0 || artifact.SizeBytes > limits.OutputBytes-size {
			return invalid("invalid output type, digest or size")
		}
		size += artifact.SizeBytes
		seen[artifact.Path] = true
	}
	return nil
}

func safeOutputPath(value string) bool {
	if len(value) == 0 || len(value) > 512 || path.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if !identifier.MatchString(part) {
			return false
		}
	}
	return true
}
