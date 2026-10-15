package sandbox_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	svc "cognida/internal/service/sandbox"
)

var principal = lifecycle.Principal{TenantID: 7, UserID: 11}

func digest(char string) string { return "sha256:" + strings.Repeat(char, 64) }

func profile() d.Profile {
	return d.Profile{
		ID: "controlled-python", Version: 1, Approved: true, IsolationLevel: d.ControlledDevelopment,
		Image: "registry.example.test/cognida/python@" + digest("a"),
		Isolation: d.IsolationRequirements{
			NonRoot: true, ReadOnlyRoot: true, DropCapabilities: true, NoNewPrivileges: true,
			SyscallFilter: true, IsolatedWorkspace: true, ReadOnlyInputs: true, NetworkDisabled: true,
		},
		MaxResources: d.ResourceLimits{
			CPUMillis: 2000, MemoryBytes: 128 << 20, DiskBytes: 16 << 20, Processes: 32,
			TimeoutSeconds: 60, InputBytes: 1 << 20, OutputBytes: 2 << 20, OutputFiles: 10, LogBytes: 1 << 20,
		},
		AllowedMediaTypes: []string{"application/json", "text/csv"},
	}
}

func request() d.SubmitRequest {
	p := profile()
	return d.SubmitRequest{
		SchemaVersion: d.SchemaVersion, IdempotencyKey: "sample:1", ProfileID: p.ID,
		ProfileVersion: p.Version, ImageSHA256: digest("a"), Resources: p.MaxResources,
		Code:   d.InputRef{Name: "code", Ref: lifecycle.RefID{AssetID: "966f17da-d49a-45c1-aacf-df771fd9a4b1", Version: 1}, SHA256: digest("b"), SizeBytes: 50},
		Inputs: []d.InputRef{{Name: "records", Ref: lifecycle.RefID{AssetID: "b8221b4d-676d-45fa-8b7d-930806d1b44e", Version: 3}, SHA256: digest("c"), SizeBytes: 100}},
	}
}

// This replaces the authorization dependency only; it never runs user code.
func authorized(_ context.Context, p lifecycle.Principal, ref lifecycle.RefID) (d.InputMetadata, error) {
	for _, input := range append([]d.InputRef{request().Code}, request().Inputs...) {
		if ref == input.Ref {
			return d.InputMetadata{TenantID: p.TenantID, Ref: ref, SHA256: input.SHA256, SizeBytes: input.SizeBytes}, nil
		}
	}
	return d.InputMetadata{}, lifecycle.ErrNotFound
}

func validator(t *testing.T) *svc.Validator {
	t.Helper()
	v, err := svc.NewValidator([]d.Profile{profile()}, authorized)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func validated(t *testing.T) svc.ValidatedSubmission {
	t.Helper()
	got, err := validator(t).ValidateSubmission(context.Background(), principal, request())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTrustedProfileRequired(t *testing.T) {
	if _, err := svc.NewValidator([]d.Profile{profile()}, nil); !errors.Is(err, d.ErrUnavailable) {
		t.Fatalf("missing authorizer: %v", err)
	}
	if _, err := svc.NewValidator(nil, authorized); !errors.Is(err, d.ErrInvalid) {
		t.Fatalf("missing profiles: %v", err)
	}
	if _, err := svc.NewValidator([]d.Profile{profile(), profile()}, authorized); !errors.Is(err, d.ErrConflict) {
		t.Fatalf("duplicate version: %v", err)
	}
	tests := []struct {
		name   string
		change func(*d.Profile)
		want   error
	}{
		{"unapproved", func(p *d.Profile) { p.Approved = false }, d.ErrInvalid},
		{"empty isolation", func(p *d.Profile) { p.IsolationLevel = "" }, d.ErrUnsupported},
		{"unsupported multitenant claim", func(p *d.Profile) { p.IsolationLevel = "untrusted_multitenant" }, d.ErrUnsupported},
		{"root", func(p *d.Profile) { p.Isolation.NonRoot = false }, d.ErrUnsupported},
		{"writable root", func(p *d.Profile) { p.Isolation.ReadOnlyRoot = false }, d.ErrUnsupported},
		{"capabilities", func(p *d.Profile) { p.Isolation.DropCapabilities = false }, d.ErrUnsupported},
		{"new privileges", func(p *d.Profile) { p.Isolation.NoNewPrivileges = false }, d.ErrUnsupported},
		{"unfiltered syscalls", func(p *d.Profile) { p.Isolation.SyscallFilter = false }, d.ErrUnsupported},
		{"shared workspace", func(p *d.Profile) { p.Isolation.IsolatedWorkspace = false }, d.ErrUnsupported},
		{"mutable inputs", func(p *d.Profile) { p.Isolation.ReadOnlyInputs = false }, d.ErrUnsupported},
		{"network", func(p *d.Profile) { p.Isolation.NetworkDisabled = false }, d.ErrUnsupported},
		{"tag", func(p *d.Profile) { p.Image = "registry.example.test/python:latest" }, d.ErrInvalid},
		{"implicit registry", func(p *d.Profile) { p.Image = "cognida/python@" + digest("a") }, d.ErrInvalid},
		{"registry credentials", func(p *d.Profile) { p.Image = "user:secret@registry.example.test/python@" + digest("a") }, d.ErrInvalid},
		{"host image", func(p *d.Profile) { p.Image = "/tmp/image@" + digest("a") }, d.ErrInvalid},
		{"registry invalid port", func(p *d.Profile) { p.Image = "localhost:99999/python@" + digest("a") }, d.ErrInvalid},
		{"image path traversal", func(p *d.Profile) { p.Image = "registry.example.test/../python@" + digest("a") }, d.ErrInvalid},
		{"unlimited time", func(p *d.Profile) { p.MaxResources.TimeoutSeconds = 0 }, d.ErrInvalid},
		{"overflow resource", func(p *d.Profile) { p.MaxResources.InputBytes = math.MaxInt64 }, d.ErrInvalid},
		{"disk insufficient", func(p *d.Profile) { p.MaxResources.DiskBytes = 1 }, d.ErrInvalid},
		{"no output policy", func(p *d.Profile) { p.AllowedMediaTypes = nil }, d.ErrInvalid},
		{"wildcard output", func(p *d.Profile) { p.AllowedMediaTypes = []string{"*/*"} }, d.ErrInvalid},
		{"mime parameters", func(p *d.Profile) { p.AllowedMediaTypes = []string{"text/csv; charset=utf-8"} }, d.ErrInvalid},
		{"mime duplicate", func(p *d.Profile) { p.AllowedMediaTypes = []string{"text/csv", "text/csv"} }, d.ErrInvalid},
		{"mime without subtype", func(p *d.Profile) { p.AllowedMediaTypes = []string{"csv"} }, d.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := profile()
			tt.change(&p)
			if _, err := svc.NewValidator([]d.Profile{p}, authorized); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSubmissionAuthorizationAndImmutableSnapshot(t *testing.T) {
	p := profile()
	var refs []lifecycle.RefID
	v, err := svc.NewValidator([]d.Profile{p}, func(ctx context.Context, owner lifecycle.Principal, ref lifecycle.RefID) (d.InputMetadata, error) {
		if owner != principal {
			t.Fatal("authorizer lost authenticated scope")
		}
		refs = append(refs, ref)
		return authorized(ctx, owner, ref)
	})
	if err != nil {
		t.Fatal(err)
	}
	p.AllowedMediaTypes[0] = "application/x-secret"
	r := request()
	got, err := v.ValidateSubmission(context.Background(), principal, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0] != r.Code.Ref || refs[1] != r.Inputs[0].Ref {
		t.Fatalf("not all inputs authorized: %v", refs)
	}
	if got.Binding.ID == "" || got.Binding.RequestSHA256 == "" || got.Binding.ID != got.Request.ExecutionID {
		t.Fatal("missing binding")
	}
	if got.Profile.AllowedMediaTypes[0] != "application/json" {
		t.Fatal("trusted profile mutated after construction")
	}
	if r.ExecutionID != "" {
		t.Fatal("validation mutated caller request")
	}
	r.Inputs[0].Name = "changed"
	if got.Request.Inputs[0].Name != "records" {
		t.Fatal("request snapshot aliases caller")
	}
	got.Profile.AllowedMediaTypes[0] = "application/x-secret"
	again, err := v.ValidateSubmission(context.Background(), principal, request())
	if err != nil || again.Profile.AllowedMediaTypes[0] != "application/json" {
		t.Fatal("returned snapshot aliases trusted config")
	}
}

func TestIdempotencyBindingAndOwnership(t *testing.T) {
	v := validator(t)
	ctx := context.Background()
	first := validated(t)
	r := request()
	r.ExecutionID = first.Binding.ID
	again, err := v.ValidateSubmission(ctx, principal, r)
	if err != nil || svc.ValidateReplay(principal, first.Binding, again.Binding) != nil {
		t.Fatalf("same request replay rejected: %v", err)
	}
	r.Resources.TimeoutSeconds--
	changed, err := v.ValidateSubmission(ctx, principal, r)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Binding.ID != first.Binding.ID || changed.Binding.RequestSHA256 == first.Binding.RequestSHA256 {
		t.Fatal("key must retain identity while content changes hash")
	}
	if err := svc.ValidateReplay(principal, first.Binding, changed.Binding); !errors.Is(err, d.ErrConflict) {
		t.Fatalf("changed content replay: %v", err)
	}
	for _, other := range []lifecycle.Principal{{TenantID: 8, UserID: 11}, {TenantID: 7, UserID: 12}, {}} {
		if err := svc.ValidateExecutionAccess(other, first.Binding); !errors.Is(err, d.ErrForbidden) {
			t.Fatalf("cross scope access: %v", err)
		}
		if err := svc.ValidateReplay(other, first.Binding, first.Binding); !errors.Is(err, d.ErrForbidden) {
			t.Fatalf("cross scope replay: %v", err)
		}
	}
	other := lifecycle.Principal{TenantID: 8, UserID: 11}
	r = request()
	r.ExecutionID = first.Binding.ID
	if _, err := v.ValidateSubmission(ctx, other, r); !errors.Is(err, d.ErrConflict) {
		t.Fatalf("caller-selected foreign ID accepted: %v", err)
	}
	r.ExecutionID = ""
	scoped, err := v.ValidateSubmission(ctx, other, r)
	if err != nil || scoped.Binding.ID == first.Binding.ID {
		t.Fatalf("scope not bound to ID: %v", err)
	}
	for _, mutate := range []func(*d.Binding){func(b *d.Binding) { b.ID = "arbitrary" }, func(b *d.Binding) { b.RequestSHA256 = "bad" }, func(b *d.Binding) { b.IdempotencyKey = "../key" }} {
		bad := first.Binding
		mutate(&bad)
		if err := svc.ValidateExecutionAccess(principal, bad); !errors.Is(err, d.ErrConflict) {
			t.Fatalf("invalid binding: %v", err)
		}
	}
	p := profile()
	p.MaxResources.TimeoutSeconds++
	v2, err := svc.NewValidator([]d.Profile{p}, authorized)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := v2.ValidateSubmission(ctx, principal, request())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateReplay(principal, first.Binding, updated.Binding); !errors.Is(err, d.ErrConflict) {
		t.Fatal("operator policy changed under same version without binding change")
	}
}

func TestSubmissionRejectsInvalidContract(t *testing.T) {
	tests := []struct {
		name   string
		change func(*d.SubmitRequest)
		want   error
	}{
		{"schema", func(r *d.SubmitRequest) { r.SchemaVersion = "2" }, d.ErrInvalid},
		{"missing key", func(r *d.SubmitRequest) { r.IdempotencyKey = "" }, d.ErrInvalid},
		{"unknown profile", func(r *d.SubmitRequest) { r.ProfileID = "root" }, d.ErrUnsupported},
		{"profile version", func(r *d.SubmitRequest) { r.ProfileVersion = 2 }, d.ErrUnsupported},
		{"other image", func(r *d.SubmitRequest) { r.ImageSHA256 = digest("f") }, d.ErrInvalid},
		{"malformed image digest", func(r *d.SubmitRequest) { r.ImageSHA256 = "sha256:bad" }, d.ErrInvalid},
		{"arbitrary id", func(r *d.SubmitRequest) { r.ExecutionID = "any-id" }, d.ErrConflict},
		{"code name", func(r *d.SubmitRequest) { r.Code.Name = "script" }, d.ErrInvalid},
		{"empty code", func(r *d.SubmitRequest) { r.Code.SizeBytes = 0 }, d.ErrInvalid},
		{"input path", func(r *d.SubmitRequest) { r.Inputs[0].Name = "/var/run/docker.sock" }, d.ErrInvalid},
		{"duplicate input name", func(r *d.SubmitRequest) { r.Inputs[0].Name = "code" }, d.ErrInvalid},
		{"invalid immutable ref", func(r *d.SubmitRequest) { r.Inputs[0].Ref.AssetID = "https://example.test/data" }, d.ErrInvalid},
		{"missing immutable version", func(r *d.SubmitRequest) { r.Inputs[0].Ref.Version = 0 }, d.ErrInvalid},
		{"invalid input digest", func(r *d.SubmitRequest) { r.Inputs[0].SHA256 = "abc" }, d.ErrInvalid},
		{"negative size", func(r *d.SubmitRequest) { r.Inputs[0].SizeBytes = -1 }, d.ErrInvalid},
		{"aggregate input limit", func(r *d.SubmitRequest) { r.Resources.InputBytes = 149 }, d.ErrInvalid},
		{"overflow input", func(r *d.SubmitRequest) { r.Inputs[0].SizeBytes = math.MaxInt64 }, d.ErrInvalid},
		{"unbounded processes", func(r *d.SubmitRequest) { r.Resources.Processes = 0 }, d.ErrInvalid},
		{"profile budget", func(r *d.SubmitRequest) { r.Resources.CPUMillis++ }, d.ErrInvalid},
		{"too many inputs", func(r *d.SubmitRequest) { r.Inputs = make([]d.InputRef, svc.MaxInputs+1) }, d.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := request()
			tt.change(&r)
			if _, err := validator(t).ValidateSubmission(context.Background(), principal, r); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestInputAuthorizationFailsClosedAndSanitizesErrors(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*d.InputMetadata)
		failure error
		want    error
	}{
		{"cross tenant", func(m *d.InputMetadata) { m.TenantID++ }, nil, d.ErrForbidden},
		{"different ref", func(m *d.InputMetadata) { m.Ref.Version++ }, nil, d.ErrForbidden},
		{"changed bytes", func(m *d.InputMetadata) { m.SHA256 = digest("d") }, nil, d.ErrConflict},
		{"changed size", func(m *d.InputMetadata) { m.SizeBytes++ }, nil, d.ErrConflict},
		{"revoked", nil, lifecycle.ErrForbidden, d.ErrForbidden},
		{"unknown", nil, lifecycle.ErrNotFound, d.ErrForbidden},
		{"sandbox denial", nil, d.ErrForbidden, d.ErrForbidden},
		{"dependency failure", nil, errors.New("storage failed: secret-token@internal-host"), d.ErrUnavailable},
		{"cancelled resolver", nil, context.Canceled, context.Canceled},
		{"deadline resolver", nil, context.DeadlineExceeded, context.DeadlineExceeded},
		{"wrapped cancel sanitized", nil, fmt.Errorf("storage secret-token: %w", context.Canceled), context.Canceled},
		{"wrapped deadline sanitized", nil, fmt.Errorf("storage secret-token: %w", context.DeadlineExceeded), context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := svc.NewValidator([]d.Profile{profile()}, func(ctx context.Context, p lifecycle.Principal, ref lifecycle.RefID) (d.InputMetadata, error) {
				m, _ := authorized(ctx, p, ref)
				if tt.change != nil {
					tt.change(&m)
				}
				return m, tt.failure
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = v.ValidateSubmission(context.Background(), principal, request())
			if !errors.Is(err, tt.want) || strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("unsafe/wrong error: %v", err)
			}
		})
	}
}

func TestUnavailableIdentityAndCancellation(t *testing.T) {
	var missing *svc.Validator
	if _, err := missing.ValidateSubmission(context.Background(), principal, request()); !errors.Is(err, d.ErrUnavailable) {
		t.Fatalf("nil validator: %v", err)
	}
	v, err := svc.NewValidator([]d.Profile{profile()}, func(context.Context, lifecycle.Principal, lifecycle.RefID) (d.InputMetadata, error) {
		t.Fatal("unexpected authorization after rejection")
		return d.InputMetadata{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.ValidateSubmission(context.Background(), lifecycle.Principal{}, request()); !errors.Is(err, d.ErrForbidden) {
		t.Fatalf("missing identity: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.ValidateSubmission(ctx, principal, request()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	v, err = svc.NewValidator([]d.Profile{profile()}, func(ctx context.Context, p lifecycle.Principal, ref lifecycle.RefID) (d.InputMetadata, error) {
		cancel()
		return authorized(ctx, p, ref)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.ValidateSubmission(ctx, principal, request()); !errors.Is(err, context.Canceled) {
		t.Fatalf("authorization finished after cancellation: %v", err)
	}
}
