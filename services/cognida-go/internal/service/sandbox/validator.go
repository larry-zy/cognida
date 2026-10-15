// Package sandbox validates contracts without executing code or inspecting a
// runtime. Successful validation does not mean an isolation profile is deployed.
package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"regexp"
	"strconv"
	"strings"

	"cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	"github.com/google/uuid"
)

const MaxInputs = 32

var (
	identifier     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	idempotencyKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	digestPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	imagePattern   = regexp.MustCompile(`^([a-z0-9]+(?:[.-][a-z0-9]+)*)(?::([0-9]{1,5}))?/[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*@sha256:[a-f0-9]{64}$`)
)

// InputAuthorizer must resolve current authorization and immutable metadata.
// It is injected by the composition root, never selected by the requester.
type InputAuthorizer func(context.Context, lifecycle.Principal, lifecycle.RefID) (d.InputMetadata, error)

type Validator struct {
	profiles  map[string]d.Profile
	authorize InputAuthorizer
}

// ValidatedSubmission is a contract snapshot, not an execution or asset permit.
type ValidatedSubmission struct {
	Binding d.Binding
	Request d.SubmitRequest
	Profile d.Profile
}

func profileKey(id string, version int) string { return fmt.Sprintf("%s:%d", id, version) }

func NewValidator(profiles []d.Profile, authorize InputAuthorizer) (*Validator, error) {
	if authorize == nil {
		return nil, d.ErrUnavailable
	}
	if len(profiles) == 0 || len(profiles) > 64 {
		return nil, invalid("trusted profiles required")
	}
	v := &Validator{profiles: make(map[string]d.Profile), authorize: authorize}
	for _, profile := range profiles {
		if err := validateProfile(profile); err != nil {
			return nil, err
		}
		key := profileKey(profile.ID, profile.Version)
		if _, exists := v.profiles[key]; exists {
			return nil, d.ErrConflict
		}
		profile.AllowedMediaTypes = append([]string(nil), profile.AllowedMediaTypes...)
		v.profiles[key] = profile
	}
	return v, nil
}

func validateProfile(p d.Profile) error {
	if !identifier.MatchString(p.ID) || p.Version < 1 || !p.Approved {
		return invalid("explicitly approved profile required")
	}
	if p.IsolationLevel != d.ControlledDevelopment {
		return fmt.Errorf("%w: only controlled development is defined", d.ErrUnsupported)
	}
	i := p.Isolation
	if !i.NonRoot || !i.ReadOnlyRoot || !i.DropCapabilities || !i.NoNewPrivileges || !i.SyscallFilter || !i.IsolatedWorkspace || !i.ReadOnlyInputs || !i.NetworkDisabled {
		return fmt.Errorf("%w: all isolation requirements are mandatory", d.ErrUnsupported)
	}
	imageParts := imagePattern.FindStringSubmatch(p.Image)
	if imageParts == nil || (!strings.Contains(imageParts[1], ".") && imageParts[1] != "localhost" && imageParts[2] == "") {
		return invalid("trusted fully qualified digest-pinned image required")
	}
	if imageParts[2] != "" {
		port, _ := strconv.Atoi(imageParts[2])
		if port < 1 || port > 65535 {
			return invalid("invalid trusted registry port")
		}
	}
	if err := validateLimits(p.MaxResources); err != nil {
		return err
	}
	if len(p.AllowedMediaTypes) == 0 || len(p.AllowedMediaTypes) > 32 {
		return invalid("bounded output media allowlist required")
	}
	seen := map[string]bool{}
	for _, value := range p.AllowedMediaTypes {
		parsed, params, err := mime.ParseMediaType(value)
		if err != nil || len(value) > 128 || parsed != value || strings.Count(parsed, "/") != 1 || len(params) != 0 || strings.Contains(value, "*") || seen[value] {
			return invalid("invalid output media allowlist")
		}
		seen[value] = true
	}
	return nil
}

// Conservative absolute ceilings bound metadata arithmetic, not runtime support.
// A future runner must reject limits it cannot actually enforce.
func validateLimits(r d.ResourceLimits) error {
	values := []int64{r.CPUMillis, r.MemoryBytes, r.DiskBytes, r.Processes, r.TimeoutSeconds, r.InputBytes, r.OutputBytes, r.OutputFiles, r.LogBytes}
	ceilings := []int64{64000, 64 << 30, 128 << 30, 1024, 3600, 1 << 30, 1 << 30, 1000, 64 << 20}
	for i, value := range values {
		if value <= 0 || value > ceilings[i] {
			return invalid("resource budgets must be positive and bounded")
		}
	}
	if r.InputBytes+r.OutputBytes+r.LogBytes > r.DiskBytes {
		return invalid("disk budget must cover input, output and logs")
	}
	return nil
}

func withinLimits(r, max d.ResourceLimits) bool {
	return r.CPUMillis <= max.CPUMillis && r.MemoryBytes <= max.MemoryBytes && r.DiskBytes <= max.DiskBytes && r.Processes <= max.Processes && r.TimeoutSeconds <= max.TimeoutSeconds && r.InputBytes <= max.InputBytes && r.OutputBytes <= max.OutputBytes && r.OutputFiles <= max.OutputFiles && r.LogBytes <= max.LogBytes
}

func executionID(p lifecycle.Principal, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("sandbox:v1:%d:%d:%s", p.TenantID, p.UserID, key))).String()
}

func (v *Validator) ValidateSubmission(ctx context.Context, p lifecycle.Principal, req d.SubmitRequest) (ValidatedSubmission, error) {
	var empty ValidatedSubmission
	if p.Validate() != nil {
		return empty, d.ErrForbidden
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if v == nil || v.authorize == nil {
		return empty, d.ErrUnavailable
	}
	if req.SchemaVersion != d.SchemaVersion || !idempotencyKey.MatchString(req.IdempotencyKey) || len(req.Inputs) > MaxInputs {
		return empty, invalid("invalid submission identity or input count")
	}
	profile, ok := v.profiles[profileKey(req.ProfileID, req.ProfileVersion)]
	if !ok {
		return empty, d.ErrUnsupported
	}
	if !digestPattern.MatchString(req.ImageSHA256) || !strings.HasSuffix(profile.Image, "@"+req.ImageSHA256) {
		return empty, invalid("image digest differs from trusted profile")
	}
	if err := validateLimits(req.Resources); err != nil {
		return empty, err
	}
	if !withinLimits(req.Resources, profile.MaxResources) {
		return empty, invalid("requested resources exceed profile")
	}
	id := executionID(p, req.IdempotencyKey)
	if req.ExecutionID != "" && req.ExecutionID != id {
		return empty, d.ErrConflict
	}
	req.ExecutionID = id
	if req.Code.Name != "code" || req.Code.SizeBytes <= 0 {
		return empty, invalid("nonempty code asset named code required")
	}
	inputs := append([]d.InputRef{req.Code}, req.Inputs...)
	seen := map[string]bool{}
	var total int64
	for _, input := range inputs {
		if !identifier.MatchString(input.Name) || seen[input.Name] || input.Ref.Validate() != nil || !digestPattern.MatchString(input.SHA256) || input.SizeBytes < 0 || input.SizeBytes > req.Resources.InputBytes-total {
			return empty, invalid("invalid or over-budget immutable input")
		}
		seen[input.Name] = true
		total += input.SizeBytes
		actual, err := v.authorize(ctx, p, input.Ref)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return empty, context.Canceled
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return empty, context.DeadlineExceeded
			}
			if errors.Is(err, d.ErrForbidden) || errors.Is(err, lifecycle.ErrForbidden) || errors.Is(err, lifecycle.ErrNotFound) {
				return empty, d.ErrForbidden
			}
			return empty, d.ErrUnavailable // Do not expose resolver errors containing storage URLs or secrets.
		}
		if actual.TenantID != p.TenantID || actual.Ref != input.Ref {
			return empty, d.ErrForbidden
		}
		if actual.SHA256 != input.SHA256 || actual.SizeBytes != input.SizeBytes {
			return empty, d.ErrConflict
		}
		if err := ctx.Err(); err != nil {
			return empty, err
		}
	}
	profile.AllowedMediaTypes = append([]string(nil), profile.AllowedMediaTypes...)
	req.Inputs = append([]d.InputRef(nil), req.Inputs...)
	// Bind the trusted profile snapshot as well: changing operator configuration
	// under the same version cannot silently reuse an old execution identity.
	digest := submissionDigest(p, req, profile)
	binding := d.Binding{ExecutionRef: d.ExecutionRef{ID: id, TenantID: p.TenantID, UserID: p.UserID, RequestSHA256: digest}, IdempotencyKey: req.IdempotencyKey}
	return ValidatedSubmission{Binding: binding, Request: req, Profile: profile}, nil
}

func submissionDigest(p lifecycle.Principal, req d.SubmitRequest, profile d.Profile) string {
	// All fields are concrete JSON-compatible scalar/struct/slice types.
	payload, _ := json.Marshal(struct {
		Principal lifecycle.Principal
		Request   d.SubmitRequest
		Profile   d.Profile
	}{p, req, profile})
	hash := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ValidateExecutionAccess checks ownership and binding shape only. A future
// controller must additionally load the stored binding and recheck policy.
func ValidateExecutionAccess(p lifecycle.Principal, binding d.Binding) error {
	if p.Validate() != nil || binding.TenantID != p.TenantID || binding.UserID != p.UserID {
		return d.ErrForbidden
	}
	if !idempotencyKey.MatchString(binding.IdempotencyKey) || binding.ID != executionID(p, binding.IdempotencyKey) || !digestPattern.MatchString(binding.RequestSHA256) {
		return d.ErrConflict
	}
	return nil
}

// ValidateReplay compares a stored binding to the newly authorized request. It
// performs no write or lock; the future repository must make this check atomic.
func ValidateReplay(p lifecycle.Principal, stored, requested d.Binding) error {
	if err := ValidateExecutionAccess(p, stored); err != nil {
		return err
	}
	if err := ValidateExecutionAccess(p, requested); err != nil {
		return err
	}
	if stored != requested {
		return d.ErrConflict
	}
	return nil
}

func invalid(reason string) error { return fmt.Errorf("%w: %s", d.ErrInvalid, reason) }
