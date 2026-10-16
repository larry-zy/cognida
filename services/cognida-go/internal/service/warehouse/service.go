package warehouse

import (
	"bytes"
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	lin "cognida/internal/model/lineage"
	sem "cognida/internal/model/semantic"
	d "cognida/internal/model/warehouse"
	"cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	Repo      d.Repository
	Assets    *lifecycle.Service
	Semantic  sem.Repository
	Lineage   lin.Repository
	Authorize func(context.Context, l.Principal, string) error
}

func NewService(repo d.Repository, assets *lifecycle.Service, semantic sem.Repository, lineage lin.Repository, auth func(context.Context, l.Principal, string) error) *Service {
	return &Service{repo, assets, semantic, lineage, auth}
}
func (s *Service) access(ctx context.Context, p l.Principal, permission string) error {
	if p.Validate() != nil || s.Authorize == nil {
		return d.ErrForbidden
	}
	return s.Authorize(ctx, p, permission)
}
func validKey(key string) bool { return strings.TrimSpace(key) != "" && len(key) <= 100 }
func (s *Service) semantic(ctx context.Context, p l.Principal, def d.Definition, current bool) error {
	if def.SemanticModelID == "" {
		return nil
	}
	if s.Semantic == nil {
		return problem("", "semantic_unavailable", "语义模型仓储不可用，不能验证固定口径")
	}
	bundle, e := s.Semantic.GetModelBundle(ctx, def.SemanticModelID)
	if e != nil {
		return e
	}
	if bundle == nil || bundle.Model == nil || bundle.Model.TenantID != p.TenantID {
		return d.ErrForbidden
	}
	if current && bundle.Model.Version != def.SemanticVersion {
		return problem("", "semantic_version_changed", "语义模型已变更，必须创建新的建模版本并重新审核")
	}
	return nil
}
func (s *Service) table(ctx context.Context, p l.Principal, raw string) (l.DataAssetRef, l.Input, error) {
	id, e := l.ParseRefID(raw)
	if e != nil {
		return l.DataAssetRef{}, l.Input{}, e
	}
	ref, body, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return ref, l.Input{}, e
	}
	var in l.Input
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&in) != nil {
		return ref, in, d.ErrInvalid
	}
	if _, e = indexes(in); e != nil {
		return ref, in, e
	}
	if len(ref.Provenance.Digest) != 64 {
		return ref, in, d.ErrInvalid
	}
	return ref, in, nil
}
func (s *Service) sources(ctx context.Context, p l.Principal, def d.Definition, override string) (map[string]l.Input, map[string]string, []l.RefID, error) {
	if _, e := validate(def); e != nil {
		return nil, nil, nil, e
	}
	if e := s.semantic(ctx, p, def, false); e != nil {
		return nil, nil, nil, e
	}
	tables := map[string]l.Input{}
	digests := map[string]string{}
	parents := []l.RefID{}
	for _, n := range def.Nodes {
		if n.Kind != "source" {
			continue
		}
		ref, in, e := s.table(ctx, p, n.SourceRef)
		if e != nil {
			return nil, nil, nil, e
		}
		digests[n.SourceRef] = ref.Provenance.Digest
		parents = append(parents, ref.RefID)
		if n.ID == def.FactNode && override != "" && override != n.SourceRef {
			ref, in, e = s.table(ctx, p, override)
			if e != nil {
				return nil, nil, nil, e
			}
			digests[override] = ref.Provenance.Digest
			parents = append(parents, ref.RefID)
		}
		tables[n.ID] = in
	}
	return tables, digests, parents, nil
}
func (s *Service) Preview(ctx context.Context, p l.Principal, def d.Definition, limit int) (*d.Preview, error) {
	var normalization error
	def, normalization = canonicalDefinition(def)
	if normalization != nil {
		return nil, normalization
	}
	if e := s.access(ctx, p, "read"); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	if e := s.semantic(ctx, p, def, true); e != nil {
		return nil, e
	}
	sources, hashes, _, e := s.sources(ctx, p, def, "")
	if e != nil {
		return nil, e
	}
	out, e := execute(ctx, def, sources)
	if e != nil {
		return nil, e
	}
	out.SourceDigests = hashes
	refs := []string{}
	for ref := range hashes {
		refs = append(refs, ref)
	}
	out.Impact, e = s.Repo.Impact(ctx, p, refs)
	if e != nil {
		return nil, e
	}
	visible := make([]d.Impact, 0, len(out.Impact))
	for _, impact := range out.Impact {
		if strings.HasPrefix(impact.Kind, "asset:") {
			id, e := l.ParseRefID(impact.ID)
			if e != nil {
				return nil, e
			}
			ref, _, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, id)
			if e != nil {
				visible = append(visible, d.Impact{Kind: "unavailable_asset", Unknown: true, Name: "部分依赖当前不可读取，未披露其标识"})
				continue
			}
			impact.Kind = string(ref.Kind)
			impact.Name = ref.Name
		}
		visible = append(visible, impact)
	}
	out.Impact = visible
	for _, row := range out.Output.Rows {
		exact := []string{}
		for _, value := range row {
			raw, _ := json.Marshal(value)
			exact = append(exact, string(raw))
		}
		out.ExactRows = append(out.ExactRows, exact)
	}
	if len(out.ExactRows) > limit {
		out.ExactRows = out.ExactRows[:limit]
	}
	if len(out.Output.Rows) > limit {
		out.Output.Rows = out.Output.Rows[:limit]
	}
	return out, nil
}
func (s *Service) SaveModel(ctx context.Context, p l.Principal, id string, expected int, def d.Definition, key string) (*d.Model, error) {
	var normalization error
	def, normalization = canonicalDefinition(def)
	if normalization != nil {
		return nil, normalization
	}
	if !validKey(key) || expected < 0 {
		return nil, d.ErrInvalid
	}
	if _, e := s.Preview(ctx, p, def, 1); e != nil {
		return nil, e
	}
	return s.Repo.SaveModel(ctx, &d.Model{ID: id, TenantID: p.TenantID, UserID: p.UserID, Definition: def, Digest: digest(def)}, expected, key)
}
func (s *Service) GetModel(ctx context.Context, p l.Principal, id string, version int) (*d.Model, error) {
	if e := s.access(ctx, p, "read"); e != nil {
		return nil, e
	}
	m, e := s.Repo.GetModel(ctx, p, id, version)
	if e != nil {
		return nil, e
	}
	if _, _, _, e = s.sources(ctx, p, m.Definition, ""); e != nil {
		return nil, e
	}
	return m, nil
}
func (s *Service) ListModels(ctx context.Context, p l.Principal, cursor string) ([]d.Model, string, error) {
	if e := s.access(ctx, p, "read"); e != nil {
		return nil, "", e
	}
	rows, next, e := s.Repo.ListModels(ctx, p, cursor)
	if e != nil {
		return nil, "", e
	}
	for _, m := range rows {
		if _, _, _, e = s.sources(ctx, p, m.Definition, ""); e != nil {
			return nil, "", e
		}
	}
	return rows, next, nil
}
func (s *Service) CreateRun(ctx context.Context, p l.Principal, req d.RunRequest, key string, deadline *time.Time) (*d.Run, error) {
	return s.createRun(ctx, p, req, key, deadline, "")
}
func (s *Service) createRun(ctx context.Context, p l.Principal, req d.RunRequest, key string, deadline *time.Time, commandDigest string) (*d.Run, error) {
	if !validKey(key) {
		return nil, d.ErrInvalid
	}
	prepared, e := s.prepareRun(ctx, p, req)
	if e != nil {
		return nil, e
	}
	prepared.CommandDigest = commandDigest
	prepared.Deadline = deadline
	return s.Repo.CreateRun(ctx, prepared, key)
}

// Preflight computes the bounded candidate without importing assets or persisting runs.
func (s *Service) Preflight(ctx context.Context, p l.Principal, req d.RunRequest) error {
	_, e := s.prepareRun(ctx, p, req)
	return e
}
func (s *Service) prepareRun(ctx context.Context, p l.Principal, req d.RunRequest) (*d.Run, error) {
	if req.ModelVersion < 1 {
		return nil, d.ErrInvalid
	}
	switch req.Mode {
	case "full":
		if req.EventFrom != nil || req.EventTo != nil {
			return nil, d.ErrInvalid
		}
	case "incremental":
		if req.FactSourceRef == "" || req.EventFrom != nil || req.EventTo != nil {
			return nil, d.ErrInvalid
		}
	case "backfill":
		if req.FactSourceRef == "" || req.EventFrom == nil || req.EventTo == nil || !req.EventTo.After(*req.EventFrom) {
			return nil, d.ErrInvalid
		}
	default:
		return nil, d.ErrInvalid
	}
	m, e := s.GetModel(ctx, p, req.ModelID, req.ModelVersion)
	if e != nil {
		return nil, e
	}
	head, e := s.Repo.GetModel(ctx, p, req.ModelID, 0)
	if e != nil {
		return nil, e
	}
	if head.Version != req.ModelVersion {
		return nil, problem("", "model_version_changed", "模型已产生新版本，旧版本不能新建可激活候选")
	}
	if e := s.semantic(ctx, p, m.Definition, true); e != nil {
		return nil, e
	}
	tables, hashes, _, e := s.sources(ctx, p, m.Definition, req.FactSourceRef)
	if e != nil {
		return nil, e
	}
	var base l.Input
	if req.Mode != "full" {
		if m.ActiveRunID == "" {
			return nil, problem("", "active_base_required", "必须先批准完整版本，才能增量或回填")
		}
		old, e := s.GetRun(ctx, p, m.ActiveRunID)
		if e != nil {
			return nil, e
		}
		if old.DefinitionDigest != m.Digest {
			return nil, problem("", "definition_changed", "转换版本变更后必须先生成完整候选")
		}
		_, base, e = s.table(ctx, p, old.FactRef)
		if e != nil {
			return nil, e
		}
	}
	merged, _, e := mergeFacts(m.Definition, base, tables[m.Definition.FactNode], req, m.Watermark)
	if e != nil {
		return nil, e
	}
	tables[m.Definition.FactNode] = merged
	if _, e = execute(ctx, m.Definition, tables); e != nil {
		return nil, e
	}
	return &d.Run{TenantID: p.TenantID, UserID: p.UserID, RequestID: agentctx.MustGetRequestID(ctx), Request: req, DefinitionDigest: m.Digest, SourceDigests: hashes, BaseRunID: m.ActiveRunID, Partitions: []d.Partition{}, Stages: []d.Stage{}, Lineage: []d.ColumnLineage{}}, nil
}
func (s *Service) authorizeRun(ctx context.Context, p l.Principal, r *d.Run) error {
	m, e := s.Repo.GetModel(ctx, p, r.Request.ModelID, r.Request.ModelVersion)
	if e != nil {
		return e
	}
	_, hashes, _, e := s.sources(ctx, p, m.Definition, r.Request.FactSourceRef)
	if e != nil {
		return e
	}
	if digest(hashes) != digest(r.SourceDigests) || m.Digest != r.DefinitionDigest {
		return d.ErrConflict
	}
	if r.BaseRunID != "" {
		old, e := s.Repo.GetRun(ctx, p, r.BaseRunID)
		if e != nil {
			return e
		}
		if old.FactRef == "" {
			return d.ErrConflict
		}
		if _, _, e = s.table(ctx, p, old.FactRef); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) GetRun(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	if e := s.access(ctx, p, "read"); e != nil {
		return nil, e
	}
	r, e := s.Repo.GetRun(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.authorizeRun(ctx, p, r); e != nil {
		return nil, e
	}
	return r, nil
}
func (s *Service) ListRuns(ctx context.Context, p l.Principal, model, cursor string) ([]d.Run, string, error) {
	if e := s.access(ctx, p, "read"); e != nil {
		return nil, "", e
	}
	rows, next, e := s.Repo.ListRuns(ctx, p, model, cursor)
	if e != nil {
		return nil, "", e
	}
	for i := range rows {
		if e = s.authorizeRun(ctx, p, &rows[i]); e != nil {
			return nil, "", e
		}
	}
	return rows, next, nil
}
func (s *Service) Cancel(ctx context.Context, p l.Principal, id string, version int) (*d.Run, error) {
	if e := s.access(ctx, p, "read"); e != nil {
		return nil, e
	}
	r, e := s.Repo.Cancel(ctx, p, id, version)
	if e != nil {
		return nil, e
	}
	if e = s.authorizeRun(ctx, p, r); e != nil {
		return &d.Run{ID: r.ID, Status: r.Status, Version: r.Version, Error: "来源不可访问；仅返回取消状态"}, nil
	}
	return r, nil
}
func (s *Service) Approve(ctx context.Context, p l.Principal, id string, version int) (*d.Run, error) {
	if e := s.access(ctx, p, "admin"); e != nil {
		return nil, e
	}
	r, e := s.GetRun(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if r.Status != "awaiting_approval" && r.Status != "approved" {
		return nil, d.ErrConflict
	}
	if !r.LineageRegistered {
		return nil, problem("", "lineage_missing", "实际列血缘尚未登记，不能批准切换")
	}
	m, e := s.Repo.GetModel(ctx, p, r.Request.ModelID, 0)
	if e != nil {
		return nil, e
	}
	if m.Version != r.Request.ModelVersion {
		return nil, d.ErrConflict
	}
	if e := s.semantic(ctx, p, m.Definition, true); e != nil {
		return nil, e
	}
	ref, table, e := s.table(ctx, p, r.OutputRef)
	if e != nil {
		return nil, e
	}
	body, e := json.Marshal(table)
	if e != nil {
		return nil, e
	}
	published, e := s.Assets.ImportDerived(ctx, p, l.PublishedAsset, "warehouse-approved:"+r.ID, []l.RefID{ref.RefID}, body, l.PolicySnapshot{Sensitivity: ref.Policy.Sensitivity, Scopes: []string{"read"}, Approved: true, Version: r.DefinitionDigest})
	if e != nil {
		return nil, e
	}
	return s.Repo.Approve(ctx, p, id, version, published.RefID.String())
}
func (s *Service) Events(ctx context.Context, p l.Principal, id string) ([]d.Event, error) {
	if _, e := s.GetRun(ctx, p, id); e != nil {
		return nil, e
	}
	return s.Repo.Events(ctx, p, id)
}
func (s *Service) persistTable(ctx context.Context, p l.Principal, run *d.Run, suffix string, parents []l.RefID, in l.Input) (string, error) {
	raw, e := json.Marshal(in)
	if e != nil {
		return "", e
	}
	ref, e := s.Assets.ImportDerived(ctx, p, l.Result, "warehouse-candidate:"+run.ID+":"+suffix, parents, raw, l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: run.DefinitionDigest})
	return ref.RefID.String(), e
}
func (s *Service) Process(ctx context.Context, p l.Principal, id string) error {
	run, e := s.Repo.Claim(ctx, p, id)
	if e != nil || run == nil {
		return e
	}
	ctx = agentctx.WithRequestID(ctx, run.RequestID)
	work := ctx
	cancel := func() {}
	if run.Deadline != nil {
		work, cancel = context.WithDeadline(ctx, *run.Deadline)
	}
	defer cancel()
	fail := func(err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cleanup, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_ = s.Repo.Update(cleanup, run, func(v *d.Run) error { v.Status = "failed"; v.Error = err.Error(); return nil })
		return err
	}
	if e = s.access(work, p, "read"); e != nil {
		return fail(e)
	}
	if e = s.authorizeRun(work, p, run); e != nil {
		return fail(e)
	}
	m, e := s.Repo.GetModel(work, p, run.Request.ModelID, run.Request.ModelVersion)
	if e != nil {
		return fail(e)
	}
	if e := s.semantic(work, p, m.Definition, true); e != nil {
		return fail(e)
	}
	tables, hashes, parents, e := s.sources(work, p, m.Definition, run.Request.FactSourceRef)
	if e != nil {
		return fail(e)
	}
	if digest(hashes) != digest(run.SourceDigests) {
		return fail(d.ErrConflict)
	}
	var base l.Input
	var watermark *time.Time
	if run.Request.Mode != "full" {
		old, e := s.Repo.GetRun(work, p, run.BaseRunID)
		if e != nil {
			return fail(e)
		}
		ref, table, e := s.table(work, p, old.FactRef)
		if e != nil {
			return fail(e)
		}
		base = table
		watermark = old.Watermark
		parents = append(parents, ref.RefID)
	}
	fact, next, e := mergeFacts(m.Definition, base, tables[m.Definition.FactNode], run.Request, watermark)
	if e != nil {
		return fail(e)
	}
	tables[m.Definition.FactNode] = fact
	computed, e := execute(work, m.Definition, tables)
	if e != nil {
		return fail(e)
	}
	// Real lineage records use the effective pinned fact snapshot, including explicit override.
	if run.Request.FactSourceRef != "" {
		for i := range computed.Lineage {
			if computed.Lineage[i].Node == m.Definition.FactNode {
				computed.Lineage[i].SourceRef = run.Request.FactSourceRef
			}
		}
	}
	factRef, e := s.persistTable(work, p, run, "facts", parents, fact)
	if e != nil {
		return fail(e)
	}
	for start, index := 0, 0; start < len(computed.Output.Rows); start, index = start+100, index+1 {
		if e = s.authorizeRun(work, p, run); e != nil {
			return fail(e)
		}
		part := computed.Output
		end := start + 100
		if end > len(part.Rows) {
			end = len(part.Rows)
		}
		part.Rows = part.Rows[start:end]
		hash := digest(part)
		if index < len(run.Partitions) {
			if run.Partitions[index].Index != index || run.Partitions[index].Digest != hash {
				return fail(d.ErrConflict)
			}
			if _, _, e = s.table(work, p, run.Partitions[index].OutputRef); e != nil {
				return fail(e)
			}
			continue
		}
		ref, e := s.persistTable(work, p, run, fmt.Sprintf("part-%d", index), parents, part)
		if e != nil {
			return fail(e)
		}
		e = s.Repo.Update(work, run, func(v *d.Run) error {
			if len(v.Partitions) != index {
				return d.ErrConflict
			}
			v.Partitions = append(v.Partitions, d.Partition{Index: index, Count: len(part.Rows), Digest: hash, OutputRef: ref, CommittedAt: time.Now().UTC()})
			return nil
		})
		if e != nil {
			return fail(e)
		}
	}
	output, e := s.persistTable(work, p, run, "output", parents, computed.Output)
	if e != nil {
		return fail(e)
	}
	if s.Lineage == nil {
		return fail(problem("", "lineage_unavailable", "血缘登记服务不可用，候选不进入可审批状态"))
	}
	graph := lineageGraph(run, computed.Lineage)
	if e = s.Lineage.Upsert(work, p.TenantID, "warehouse:"+run.ID, graph); e != nil {
		return fail(e)
	}
	e = s.Repo.Update(work, run, func(v *d.Run) error {
		v.Status = "awaiting_approval"
		v.FactRef = factRef
		v.OutputRef = output
		v.Watermark = next
		v.Stages = computed.Stages
		v.Lineage = computed.Lineage
		v.LineageRegistered = true
		return nil
	})
	if e != nil {
		return fail(e)
	}
	return nil
}
func lineageGraph(run *d.Run, lines []d.ColumnLineage) lin.Graph {
	g := lin.Graph{Nodes: []lin.Node{}, Edges: []lin.Edge{}}
	seen := map[string]bool{}
	add := func(id, kind, name, asset string) {
		if !seen[id] {
			seen[id] = true
			g.Nodes = append(g.Nodes, lin.Node{ID: id, Kind: kind, Name: name, AssetID: asset})
		}
	}
	for _, e := range lines {
		to := digest([]string{run.ID, e.Node, e.Column})
		add(to, "WarehouseColumn", e.Node+"."+e.Column, run.ID)
		from := ""
		if e.SourceRef != "" {
			from = digest([]string{e.SourceRef, e.InputColumn})
			add(from, "AssetColumn", e.InputColumn, e.SourceRef)
		} else {
			from = digest([]string{run.ID, e.InputNode, e.InputColumn})
			add(from, "WarehouseColumn", e.InputNode+"."+e.InputColumn, run.ID)
		}
		kind := "input"
		if e.Dependency == "row_set" {
			kind = "row_set"
		}
		g.Edges = append(g.Edges, lin.Edge{From: from, To: to, Kind: kind})
	}
	return g
}
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	prefix := "warehouse-candidate:"
	approved := strings.HasPrefix(ref.ProducerID, "warehouse-approved:")
	if approved {
		prefix = "warehouse-approved:"
	}
	if !strings.HasPrefix(ref.ProducerID, prefix) {
		return d.ErrForbidden
	}
	parts := strings.Split(strings.TrimPrefix(ref.ProducerID, prefix), ":")
	if len(parts) < 1 {
		return d.ErrForbidden
	}
	r, e := s.Repo.GetRun(ctx, p, parts[0])
	if e != nil {
		return e
	}
	if e = s.access(ctx, p, "read"); e != nil {
		return e
	}
	if e = s.authorizeRun(ctx, p, r); e != nil {
		return e
	}
	target := ref.RefID.String()
	if approved {
		if r.Status == "approved" && r.PublishedRef == target {
			return nil
		}
		return d.ErrForbidden
	}
	if r.FactRef == target || r.OutputRef == target {
		return nil
	}
	for _, part := range r.Partitions {
		if part.OutputRef == target {
			return nil
		}
	}
	return d.ErrForbidden
}
func (s *Service) Requirements(ctx context.Context, p l.Principal, id string, version int) (map[string]any, error) {
	m, e := s.GetModel(ctx, p, id, version)
	if e != nil {
		return nil, e
	}
	refs := []l.RefID{}
	seen := map[string]bool{}
	for _, n := range m.Definition.Nodes {
		if n.Kind == "source" && !seen[n.SourceRef] {
			seen[n.SourceRef] = true
			id, _ := l.ParseRefID(n.SourceRef)
			refs = append(refs, id)
		}
	}
	return map[string]any{"profile": "warehouse.managed-v1", "input_refs": refs, "max_calls_per_attempt": len(refs) + 102, "max_attempts": 1, "max_timeout_seconds": 120, "model_id": id, "version": version, "target": "managed_immutable_assets", "requires_approval_to_activate": true}, nil
}

func (s *Service) SubmitGoverned(ctx context.Context, p l.Principal, req d.RunRequest, key, hash string, deadline time.Time) (*d.Run, error) {
	if p.Validate() != nil || len(hash) != 64 {
		return nil, d.ErrInvalid
	}
	if prior, e := s.LookupGoverned(ctx, p, key, hash); e != nil || prior != nil {
		return prior, e
	}
	return s.createRun(ctx, p, req, key, &deadline, hash)
}
func (s *Service) LookupGoverned(ctx context.Context, p l.Principal, key, hash string) (*d.Run, error) {
	if p.Validate() != nil || !validKey(key) || len(hash) != 64 {
		return nil, d.ErrInvalid
	}
	r, e := s.Repo.FindRunByKey(ctx, p, key)
	if e != nil || r == nil {
		return r, e
	}
	if r.CommandDigest != hash {
		return nil, d.ErrConflict
	}
	return r, nil
}
func (s *Service) CancelGoverned(ctx context.Context, p l.Principal, req d.RunRequest, key, hash string, deadline time.Time) (*d.Run, error) {
	r, e := s.LookupGoverned(ctx, p, key, hash)
	if e != nil {
		return nil, e
	}
	if r == nil {
		return s.Repo.CreateRun(ctx, &d.Run{CommandDigest: hash, TenantID: p.TenantID, UserID: p.UserID, Request: req, Deadline: &deadline, Status: "cancelled", Partitions: []d.Partition{}}, key)
	}
	if r.Status == "awaiting_approval" || r.Status == "approved" || r.Status == "failed" || r.Status == "cancelled" {
		return r, nil
	}
	return s.Repo.Cancel(ctx, p, r.ID, r.Version)
}
