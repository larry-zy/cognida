package governedops

import (
	"cognida/internal/infrastructure/job"
	ds "cognida/internal/model/datasource"
	d "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Authorizer func(context.Context, l.Principal, string) error
type Service struct {
	repo      d.Repository
	assets    *lifecycle.Service
	queue     job.Queue
	authorize Authorizer
	readQuery func(context.Context, string) (string, error)
}

func New(repo d.Repository, assets *lifecycle.Service, q job.Queue, auth Authorizer) (*Service, error) {
	if repo == nil || assets == nil || q == nil || auth == nil {
		return nil, d.ErrInvalid
	}
	return &Service{repo: repo, assets: assets, queue: q, authorize: auth}, nil
}
func (s *Service) auth(ctx context.Context, p l.Principal, action string) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	if e := s.authorize(ctx, p, action); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return d.ErrForbidden
	}
	return nil
}
func validKey(key string) bool { return strings.TrimSpace(key) != "" && len(key) <= 200 }
func (s *Service) sources(ctx context.Context, p l.Principal, refs []l.RefID, expected map[string]string) (map[string]string, error) {
	if len(refs) < 1 || len(refs) > 64 {
		return nil, d.ErrInvalid
	}
	out := map[string]string{}
	for _, id := range refs {
		ref, _, e := s.assets.Resolve(ctx, p.TenantID, p.UserID, id)
		if e != nil {
			return nil, e
		}
		if len(ref.Provenance.Digest) != 64 {
			return nil, d.ErrInvalid
		}
		out[id.String()] = ref.Provenance.Digest
		if expected != nil && expected[id.String()] != ref.Provenance.Digest {
			return nil, d.ErrConflict
		}
	}
	return out, nil
}
func tableFromInput(in l.Input) (d.Table, error) {
	if in.Kind != "table" {
		return d.Table{}, d.ErrInvalid
	}
	t := d.Table{Columns: in.Columns, Types: map[string]string{}, Rows: [][]d.Cell{}}
	for _, column := range in.Columns {
		t.Types[column] = "text"
		if ds.IsNumericSQLType(in.ColumnTypes[column]) {
			t.Types[column] = "number"
		} else if strings.EqualFold(in.ColumnTypes[column], "BOOLEAN") {
			t.Types[column] = "boolean"
		}
	}
	inferred := map[string]bool{}
	for _, row := range in.Rows {
		if len(row) != len(in.Columns) {
			return t, d.ErrInvalid
		}
		cells := []d.Cell{}
		for i, value := range row {
			c := d.Cell{Type: t.Types[in.Columns[i]]}
			switch v := value.(type) {
			case nil:
				c.Type = "null"
			case string:
				if in.ColumnTypes[in.Columns[i]] == "" {
					c.Type = "text"
				}
				c.Text = v
			case json.Number:
				c.Type = "number"
				c.Text = v.String()
			case bool:
				c.Type = "boolean"
				c.Text = fmt.Sprint(v)
			default:
				return t, d.ErrInvalid
			}
			if c.Type != "null" {
				prior := t.Types[in.Columns[i]]
				if !inferred[in.Columns[i]] && in.ColumnTypes[in.Columns[i]] == "" {
					t.Types[in.Columns[i]] = c.Type
				} else if c.Type != prior {
					return t, d.ErrInvalid
				}
			}
			if c.Type != "null" {
				inferred[in.Columns[i]] = true
			}
			cells = append(cells, c)
		}
		t.Rows = append(t.Rows, cells)
	}
	return t, t.Validate()
}
func tableInput(t d.Table) l.Input {
	in := l.Input{Kind: "table", Columns: t.Columns, ColumnTypes: map[string]string{}, Rows: [][]any{}}
	for _, c := range t.Columns {
		typ := "VARCHAR"
		if t.Types[c] == "number" {
			typ = "NUMERIC"
		} else if t.Types[c] == "boolean" {
			typ = "BOOLEAN"
		}
		in.ColumnTypes[c] = typ
	}
	for _, row := range t.Rows {
		cells := []any{}
		for _, c := range row {
			switch c.Type {
			case "null":
				cells = append(cells, nil)
			case "boolean":
				cells = append(cells, c.Text == "true")
			default:
				cells = append(cells, c.Text)
			}
		}
		in.Rows = append(in.Rows, cells)
	}
	return in
}
func (s *Service) Prepare(ctx context.Context, p l.Principal, req d.Request, key string) (*d.Job, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if !validKey(key) || req.Validate() != nil {
		return nil, d.ErrInvalid
	}
	prior, e := s.repo.FindByKey(ctx, p, d.Digest(key))
	if e != nil {
		return nil, e
	}
	if prior != nil {
		if d.Digest(prior.Plan.Request) != d.Digest(req) {
			return nil, d.ErrConflict
		}
		if _, e = s.sources(ctx, p, prior.Plan.SourceRefs, prior.Plan.SourceDigests); e != nil {
			return nil, e
		}
		return prior, nil
	}
	target, e := s.repo.Target(ctx, p, req.Target)
	if e != nil && !errors.Is(e, d.ErrNotFound) {
		return nil, e
	}
	if target == nil || errors.Is(e, d.ErrNotFound) {
		if req.ExpectedVersion != 0 || req.Action != "etl" {
			return nil, d.ErrNotFound
		}
		target = &d.Target{}
	}
	if target.Version != req.ExpectedVersion {
		return nil, d.ErrConflict
	}
	plan := d.Plan{Request: req, BeforeVersionID: target.ActiveVersionID, Parameters: []d.Cell{}, SourceRefs: []l.RefID{}, SourceDigests: map[string]string{}}
	var before *d.Version
	if target.Version > 0 {
		before, e = s.repo.ReadVersion(ctx, p, req.Target, target.Version)
		if e != nil {
			return nil, e
		}
		if _, e = s.sources(ctx, p, before.SourceRefs, nil); e != nil {
			return nil, e
		}
		plan.BeforeDigest = before.Digest
	}
	switch req.Action {
	case "etl":
		_, body, e := s.assets.Resolve(ctx, p.TenantID, p.UserID, *req.InputRef)
		if e != nil {
			return nil, e
		}
		var in l.Input
		if json.Unmarshal(body, &in) != nil {
			return nil, d.ErrInvalid
		}
		plan.After, e = tableFromInput(in)
		if e != nil {
			return nil, e
		}
		plan.SourceRefs = []l.RefID{*req.InputRef}
		plan.SQL = "INSERT INTO $TARGET (fixed authorized snapshot)"
		plan.EstimatedAffected = int64(len(plan.After.Rows))
	case "rollback":
		selected, e := s.repo.ReadVersion(ctx, p, req.Target, req.RollbackVersion)
		if e != nil {
			return nil, e
		}
		plan.RollbackVersionID = selected.ID
		plan.After = selected.Table
		plan.SourceRefs = selected.SourceRefs
		plan.SQL = "SWITCH ACTIVE VERSION TO RETAINED VERSION"
		plan.EstimatedAffected = int64(len(selected.Table.Rows))
	default:
		if before == nil {
			return nil, d.ErrConflict
		}
		plan.After, plan.SQL, plan.Parameters, plan.EstimatedAffected, e = d.Compile(before.Table, req)
		if e != nil {
			return nil, e
		}
		plan.SourceRefs = before.SourceRefs
	}
	plan.SourceDigests, e = s.sources(ctx, p, plan.SourceRefs, nil)
	if e != nil {
		return nil, e
	}
	plan.AfterDigest = d.Digest(plan.After)
	plan.Hash = d.Digest([]any{p.TenantID, p.UserID, plan})
	return s.repo.Prepare(ctx, p, plan, d.Digest(key))
}
func (s *Service) Read(ctx context.Context, p l.Principal, id string) (*d.Job, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	j, e := s.repo.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if _, e = s.sources(ctx, p, j.Plan.SourceRefs, j.Plan.SourceDigests); e != nil {
		return nil, e
	}
	if j.Status == "completed" && j.OutputRef == nil {
		if e = s.attach(ctx, p, j); e != nil {
			return nil, e
		}
		return s.repo.Get(ctx, p, id)
	}
	return j, nil
}
func (s *Service) List(ctx context.Context, p l.Principal, cursor string) ([]d.Job, string, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, "", e
	}
	rows, next, e := s.repo.List(ctx, p, cursor)
	if e != nil {
		return nil, "", e
	}
	out := []d.Job{}
	for _, j := range rows {
		if _, e = s.sources(ctx, p, j.Plan.SourceRefs, j.Plan.SourceDigests); e == nil {
			out = append(out, j)
		} else if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
	}
	return out, next, nil
}
func (s *Service) Targets(ctx context.Context, p l.Principal) ([]d.Target, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	return s.repo.Targets(ctx, p)
}
func (s *Service) Version(ctx context.Context, p l.Principal, name string, version int64) (*d.Version, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	v, e := s.repo.ReadVersion(ctx, p, name, version)
	if e != nil {
		return nil, e
	}
	if _, e = s.sources(ctx, p, v.SourceRefs, nil); e != nil {
		return nil, e
	}
	return v, nil
}

type delivery struct {
	TenantID int64  `json:"tenant_id"`
	UserID   int64  `json:"user_id"`
	ID       string `json:"id"`
}

func makeJob(j *d.Job) (job.Job, error) {
	return job.New(j.ID, d.JobType, delivery{j.TenantID, j.UserID, j.ID})
}
func (s *Service) Confirm(ctx context.Context, p l.Principal, id string, in d.Confirmation) (*d.Job, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if _, e := s.Read(ctx, p, id); e != nil {
		return nil, e
	}
	j, e := s.repo.Confirm(ctx, p, id, in)
	if e != nil {
		return nil, e
	}
	if !j.Terminal() {
		message, _ := makeJob(j)
		if e = s.queue.Enqueue(ctx, message); e != nil {
			return j, d.ErrUncertain
		}
	}
	return j, nil
}
func (s *Service) Cancel(ctx context.Context, p l.Principal, id string) (*d.Job, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	j, e := s.repo.Cancel(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.auth(ctx, p, "read"); e == nil {
		_, e = s.sources(ctx, p, j.Plan.SourceRefs, j.Plan.SourceDigests)
	}
	if e != nil {
		return &d.Job{ID: j.ID, Status: j.Status, Version: j.Version, CommittedVersion: j.CommittedVersion, AffectedRows: j.AffectedRows, Error: "source inaccessible; only durable operation state is visible"}, nil
	}
	return j, nil
}
func (s *Service) attach(ctx context.Context, p l.Principal, j *d.Job) error {
	if j.Status != "completed" {
		return d.ErrConflict
	}
	if _, e := s.sources(ctx, p, j.Plan.SourceRefs, j.Plan.SourceDigests); e != nil {
		return e
	}
	v, e := s.repo.ReadVersion(ctx, p, j.Plan.Request.Target, j.CommittedVersion)
	if e != nil {
		return e
	}
	body, _ := json.Marshal(tableInput(v.Table))
	ref, e := s.assets.ImportDerived(ctx, p, l.Result, "governed-operation:"+j.ID, j.Plan.SourceRefs, body, l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: j.Plan.Hash})
	if e != nil {
		return e
	}
	return s.repo.AttachOutput(ctx, p, j.ID, ref.RefID)
}
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	prefix := "governed-operation:"
	if !strings.HasPrefix(ref.ProducerID, prefix) {
		return d.ErrForbidden
	}
	j, e := s.repo.Get(ctx, p, strings.TrimPrefix(ref.ProducerID, prefix))
	if e != nil {
		return e
	}
	if j.Status != "completed" || j.OutputRef == nil || *j.OutputRef != ref.RefID {
		return d.ErrForbidden
	}
	if e = s.auth(ctx, p, "read"); e != nil {
		return e
	}
	_, e = s.sources(ctx, p, j.Plan.SourceRefs, j.Plan.SourceDigests)
	return e
}
func (s *Service) Handle(ctx context.Context, message job.Job) error {
	var payload delivery
	if json.Unmarshal(message.Payload, &payload) != nil || payload.ID != message.ID || message.Type != d.JobType {
		return d.ErrInvalid
	}
	p := l.Principal{TenantID: payload.TenantID, UserID: payload.UserID}
	j, e := s.repo.Claim(ctx, p, payload.ID)
	if e != nil || j == nil {
		return e
	}
	work, cancel := context.WithDeadline(ctx, j.Deadline)
	defer cancel()
	if e = s.auth(work, p, "admin"); e == nil {
		_, e = s.sources(work, p, j.Plan.SourceRefs, j.Plan.SourceDigests)
	}
	var completed *d.Job
	if e == nil {
		completed, e = s.repo.Commit(work, p, j.ID, j.Epoch)
	}
	if e != nil {
		// The lock waits for any uncertain commit before assigning failure. If the
		// transaction committed, its completed receipt wins and is never overwritten.
		reconcile, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		truth, x := s.repo.Fail(reconcile, p, j.ID, j.Epoch, "operation failed before an active version was committed")
		if x != nil {
			return d.ErrUncertain
		}
		if truth.Status == "completed" {
			return s.attach(reconcile, p, truth)
		}
		return nil
	}
	return s.attach(work, p, completed)
}
func (s *Service) PendingJobs(ctx context.Context, limit int) ([]job.Job, error) {
	rows, e := s.repo.Pending(ctx, limit)
	if e != nil {
		return nil, e
	}
	out := []job.Job{}
	for i := range rows {
		message, e := makeJob(&rows[i])
		if e != nil {
			return nil, e
		}
		out = append(out, message)
	}
	return out, nil
}
func (s *Service) Exhausted(ctx context.Context, message job.Job, _ error) error {
	return s.Handle(ctx, message)
}
func (s *Service) PrepareResultETL(ctx context.Context, p l.Principal, target, resultID, key string, replace bool) (*d.Job, error) {
	return s.PrepareAgentETL(ctx, p, target, resultID, "", key, replace)
}
