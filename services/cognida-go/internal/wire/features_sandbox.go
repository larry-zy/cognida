package wire

import (
	"cognida/internal/handler"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/sandbox"
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
)

func (f *FeatureServices) configureSandbox(db *gorm.DB) error {
	f.SandboxHandler = handler.NewSandboxHandler(nil, nil)
	if os.Getenv("SANDBOX_ENABLED") != "true" {
		return nil
	}
	if os.Getenv("SANDBOX_PROFILE_ACCEPTED") != "true" {
		return fmt.Errorf("SANDBOX_PROFILE_ACCEPTED must confirm controlled-development runtime acceptance")
	}
	allowed := map[string]bool{}
	for _, entry := range strings.Split(os.Getenv("SANDBOX_ALLOWED_PRINCIPALS"), ",") {
		allowed[strings.TrimSpace(entry)] = true
	}
	if len(allowed) == 1 && allowed[""] {
		return fmt.Errorf("Sandbox requires explicit controlled-development principals")
	}
	root := os.Getenv("SANDBOX_STAGING_DIR")
	if !filepath.IsAbs(root) || root == "/" {
		return fmt.Errorf("Sandbox staging directory must be an explicit absolute directory")
	}
	profile := d.Profile{ID: "python-controlled-development", Version: 1, Approved: true, IsolationLevel: d.ControlledDevelopment, Image: os.Getenv("SANDBOX_IMAGE"), Isolation: d.IsolationRequirements{NonRoot: true, ReadOnlyRoot: true, DropCapabilities: true, NoNewPrivileges: true, SyscallFilter: true, IsolatedWorkspace: true, ReadOnlyInputs: true, NetworkDisabled: true}, MaxResources: d.ResourceLimits{CPUMillis: 1000, MemoryBytes: 256 << 20, DiskBytes: 32 << 20, Processes: 32, TimeoutSeconds: 60, InputBytes: 8 << 20, OutputBytes: 8 << 20, OutputFiles: 16, LogBytes: 64 << 10}, AllowedMediaTypes: []string{"application/json", "text/plain"}}
	authorize := func(ctx context.Context, p l.Principal) error {
		if !allowed[fmt.Sprintf("%d:%d", p.TenantID, p.UserID)] {
			return d.ErrForbidden
		}
		return featureAuthorizer(db)(ctx, p, "sandbox")
	}
	load := func(ctx context.Context, p l.Principal, id l.RefID) ([]byte, error) {
		_, body, e := f.Lifecycle.Resolve(ctx, p.TenantID, p.UserID, id)
		return body, e
	}
	var runner *sandbox.DockerRunner
	validator, e := sandbox.NewValidator([]d.Profile{profile}, func(ctx context.Context, p l.Principal, id l.RefID) (d.InputMetadata, error) {
		return runner.Metadata(ctx, p, id)
	})
	if e != nil {
		return e
	}
	host := &sandbox.DockerHost{Root: root}
	runner, e = sandbox.NewDockerRunner(mysql.NewSandboxRepository(db), validator, host, load, func(ctx context.Context, p l.Principal, producer string, parents []l.RefID, body []byte) (l.RefID, error) {
		var input l.Input
		if json.Unmarshal(body, &input) != nil || input.Validate() != nil {
			input = l.Input{Kind: "text", Text: string(body)}
		}
		payload, err := json.Marshal(input)
		if err != nil {
			return l.RefID{}, err
		}
		ref, err := f.Lifecycle.ImportDerived(ctx, p, l.CleanedAsset, producer, parents, payload, l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"})
		return ref.RefID, err
	}, authorize)
	if e != nil {
		return e
	}
	f.SandboxRunner = runner
	f.SandboxHandler = handler.NewSandboxHandler(runner, &profile)
	return nil
}
