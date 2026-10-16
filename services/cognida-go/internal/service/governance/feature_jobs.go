package governance

import (
	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Service) featureJobs() (d.FeatureJobRepository, error) {
	r, ok := s.Repo.(d.FeatureJobRepository)
	if !ok {
		return nil, fmt.Errorf("feature operation repository unavailable")
	}
	return r, nil
}
func (s *Service) CreateFeatureJob(ctx context.Context, p l.Principal, in d.FeatureJobRequest, key string) (*d.FeatureJob, error) {
	return s.createFeatureJob(ctx, p, in, key, "", nil)
}
func (s *Service) createFeatureJob(ctx context.Context, p l.Principal, in d.FeatureJobRequest, key, operationID string, deadline *time.Time) (*d.FeatureJob, error) {
	if s.authorize(ctx, p, "read") != nil {
		return nil, d.ErrForbidden
	}
	if !validFeatureKey(key) {
		return nil, d.ErrInvalid
	}
	switch in.Mode {
	case "full", "incremental", "backfill":
	default:
		return nil, d.ErrInvalid
	}
	if in.PartitionSize == 0 {
		in.PartitionSize = 100
	}
	if in.PartitionSize < 1 || in.PartitionSize > 1000 {
		return nil, d.ErrInvalid
	}
	if in.Mode == "backfill" && (in.EventFrom == nil || in.EventTo == nil || !in.EventFrom.Before(*in.EventTo)) {
		return nil, featureErr("backfill_range", "backfill requires a nonempty [event_from,event_to) range")
	}
	if in.Mode != "backfill" && (in.EventFrom != nil || in.EventTo != nil) {
		return nil, d.ErrInvalid
	}
	f, e := s.featureVersion(ctx, p, in.FeatureID, in.FeatureVersion)
	if e != nil {
		return nil, e
	}
	if in.SourceRefID == "" {
		in.SourceRefID = f.SourceRefID
	}
	ref, input, e := s.input(ctx, p, in.SourceRefID, false)
	if e != nil {
		return nil, e
	}
	if input.Kind != "table" || len(input.Rows) > 10000 {
		return nil, featureErr("input_limit", "materialization snapshot must contain at most 10000 rows; no truncation is performed")
	}
	def, e := parseFeatureDefinition(f.Definition)
	if e != nil {
		return nil, e
	}
	if in.Mode != "full" && def.EventTimeField == "" {
		return nil, featureErr("event_time_required", "incremental and backfill require an explicit event time field")
	}
	if def.Kind == "embedding" {
		if e = s.checkFeatureEmbedding(def); e != nil {
			return nil, e
		}
		if in.PartitionSize > 32 {
			in.PartitionSize = 32
		}
	}
	raw, _ := json.Marshal(def)
	if len(ref.Provenance.Digest) != 64 {
		return nil, featureErr("source_digest", "source must have a verified immutable digest")
	}
	parts := []d.FeaturePartition{}
	for start := 0; start < len(input.Rows); start += in.PartitionSize {
		parts = append(parts, d.FeaturePartition{Index: len(parts), Status: "pending"})
	}
	job := &d.FeatureJob{RequestID: agentctx.MustGetRequestID(ctx), OperationID: operationID, Deadline: deadline, TenantID: p.TenantID, UserID: p.UserID, Request: in, AnchorRefID: f.SourceRefID, DefinitionDigest: featureHash(raw), SourceDigest: ref.Provenance.Digest, Embedding: def.Kind == "embedding", TotalRows: len(input.Rows), Partitions: parts}
	r, e := s.featureJobs()
	if e != nil {
		return nil, e
	}
	return r.CreateFeatureJob(ctx, job, key)
}
func (s *Service) authorizeFeatureJob(ctx context.Context, p l.Principal, j *d.FeatureJob) error {
	if p.Validate() != nil || j.TenantID != p.TenantID || j.UserID != p.UserID || s.authorize(ctx, p, "read") != nil {
		return d.ErrForbidden
	}
	if _, e := s.featureVersion(ctx, p, j.Request.FeatureID, j.Request.FeatureVersion); e != nil {
		return e
	}
	ref, _, e := s.input(ctx, p, j.Request.SourceRefID, false)
	if e != nil {
		return e
	}
	if ref.Provenance.Digest != j.SourceDigest {
		return d.ErrConflict
	}
	return nil
}
func (s *Service) GetFeatureJob(ctx context.Context, p l.Principal, id string) (*d.FeatureJob, error) {
	r, e := s.featureJobs()
	if e != nil {
		return nil, e
	}
	j, e := r.GetFeatureJob(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return nil, e
	}
	if e = s.authorizeFeatureJob(ctx, p, j); e != nil {
		return nil, e
	}
	return j, nil
}
func (s *Service) ListFeatureJobs(ctx context.Context, p l.Principal, cursor string) ([]d.FeatureJob, string, error) {
	if s.authorize(ctx, p, "read") != nil {
		return nil, "", d.ErrForbidden
	}
	r, e := s.featureJobs()
	if e != nil {
		return nil, "", e
	}
	jobs, next, e := r.ListFeatureJobs(ctx, p.TenantID, p.UserID, cursor, 50)
	if e != nil {
		return nil, "", e
	}
	out := []d.FeatureJob{}
	for i := range jobs {
		if s.authorizeFeatureJob(ctx, p, &jobs[i]) == nil {
			out = append(out, jobs[i])
		}
	}
	return out, next, nil
}
func (s *Service) ControlFeatureJob(ctx context.Context, p l.Principal, id, action string, version int) (*d.FeatureJob, error) {
	if s.authorize(ctx, p, "read") != nil {
		return nil, d.ErrForbidden
	}
	r, e := s.featureJobs()
	if e != nil {
		return nil, e
	}
	j, e := r.GetFeatureJob(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return nil, e
	}
	switch action {
	case "cancel":
		e = r.CancelFeatureJob(ctx, p.TenantID, p.UserID, id, version)
	case "withdraw":
		if s.authorize(ctx, p, "admin") != nil {
			return nil, d.ErrForbidden
		}
		e = r.WithdrawFeatureJob(ctx, p.TenantID, p.UserID, id, version)
	case "retry":
		if e = s.authorizeFeatureJob(ctx, p, j); e != nil {
			return nil, e
		}
		e = r.RetryFeatureJob(ctx, p.TenantID, p.UserID, id, version)
	default:
		return nil, d.ErrInvalid
	}
	if e != nil {
		return nil, e
	}
	out, e := r.GetFeatureJob(ctx, p.TenantID, p.UserID, id)
	if e != nil {
		return nil, e
	}
	if s.authorizeFeatureJob(ctx, p, out) != nil {
		out.AnchorRefID = ""
		out.SourceDigest = ""
		out.DefinitionDigest = ""
		out.Request.SourceRefID = ""
		out.Error = ""
		for i := range out.Partitions {
			out.Partitions[i].OutputRefID = ""
			out.Partitions[i].Digest = ""
		}
	}
	return out, nil
}

func featureJobInclude(j *d.FeatureJob, at time.Time) bool {
	if j.Request.Mode == "incremental" && j.WatermarkBefore != nil && !at.After(*j.WatermarkBefore) {
		return false
	}
	if j.Request.Mode == "backfill" && (at.Before(*j.Request.EventFrom) || !at.Before(*j.Request.EventTo)) {
		return false
	}
	return true
}

// RunFeatureJob resumes deterministic partitions. A stale embedding lease is
// reconciled by the repository without issuing an unconfirmed paid call again.
func (s *Service) RunFeatureJob(ctx context.Context, tenant, user int64, id string) error {
	r, e := s.featureJobs()
	if e != nil {
		return e
	}
	j, e := r.ClaimFeatureJob(ctx, tenant, user, id)
	if e != nil || j == nil {
		return e
	}
	if j.Deadline != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, *j.Deadline)
		defer cancel()
	}
	if j.RequestID != "" {
		ctx = agentctx.WithRequestID(ctx, j.RequestID)
	}
	p := l.Principal{TenantID: tenant, UserID: user}
	fail := func(cause error) error {
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ctx.Err()
		}
		end, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if e := r.FinishFeatureJob(end, j, "failed", cause.Error(), nil); e != nil {
			return e
		}
		return nil
	}
	if e = s.authorizeFeatureJob(ctx, p, j); e != nil {
		return fail(e)
	}
	f, e := s.featureVersion(ctx, p, j.Request.FeatureID, j.Request.FeatureVersion)
	if e != nil {
		return fail(e)
	}
	ref, input, e := s.input(ctx, p, j.Request.SourceRefID, false)
	if e != nil {
		return fail(e)
	}
	f.SourceRefID = j.Request.SourceRefID
	def, e := parseFeatureDefinition(f.Definition)
	if e != nil {
		return fail(e)
	}
	raw, _ := json.Marshal(def)
	if featureHash(raw) != j.DefinitionDigest {
		return fail(d.ErrConflict)
	}
	all := []d.FeatureValue{}
	if !j.Embedding {
		all, _, e = computeFeatureValuesLimit(ctx, f, ref, input, 10000)
		if e != nil {
			return fail(e)
		}
	}
	var watermark *time.Time
	for index := range j.Partitions {
		if j.OperationID != "" {
			op, err := s.Repo.Get(ctx, p.TenantID, d.GovernedKind, j.OperationID)
			if err != nil {
				return fail(err)
			}
			if op.UserID != p.UserID {
				return fail(d.ErrForbidden)
			}
			if op.Status == "cancel_requested" || op.Status == "cancelled" {
				return r.FinishFeatureJob(context.WithoutCancel(ctx), j, "cancelled", "", nil)
			}
		}
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		if e = s.authorizeFeatureJob(ctx, p, j); e != nil {
			return fail(e)
		}
		if j.Partitions[index].Status == "committed" {
			continue
		}
		var values []d.FeatureValue
		start, end := index*j.Request.PartitionSize, (index+1)*j.Request.PartitionSize
		if j.Embedding {
			if end > len(input.Rows) {
				end = len(input.Rows)
			}
			batch := input
			batch.Columns = append([]string(nil), input.Columns...)
			batch.ColumnTypes = nil
			batch.Rows = nil
			localDef := def
			entityColumn := ""
			if def.EntityField == "" {
				entityColumn = "__cognida_feature_entity"
				for _, c := range batch.Columns {
					if c == entityColumn {
						return fail(d.ErrInvalid)
					}
				}
				batch.Columns = append(batch.Columns, entityColumn)
				localDef.EntityField = entityColumn
			}
			for n := start; n < end; n++ {
				row := append([]any(nil), input.Rows[n]...)
				if entityColumn != "" {
					row = append(row, fmt.Sprintf("%s:%d", ref.AssetID, n))
				}
				if def.EventTimeField != "" {
					for col, name := range input.Columns {
						if name == def.EventTimeField {
							at, err := featureEvent(row[col])
							if err != nil {
								return fail(err)
							}
							if !featureJobInclude(j, at) {
								row = nil
							}
							break
						}
					}
				}
				if row != nil {
					batch.Rows = append(batch.Rows, row)
				}
			}
			if len(batch.Rows) > 0 {
				local := f
				local.Definition, _ = json.Marshal(localDef)
				values, _, e = s.computeFeature(ctx, local, ref, batch)
				if e != nil {
					return fail(e)
				}
			}
		} else {
			if end > len(all) {
				end = len(all)
			}
			if start < len(all) {
				for _, v := range all[start:end] {
					if featureJobInclude(j, v.Timestamp) {
						values = append(values, v)
					}
				}
			}
		}
		for _, v := range values {
			if watermark == nil || v.Timestamp.After(*watermark) {
				at := v.Timestamp
				watermark = &at
			}
		}
		canonical, _ := json.Marshal(values)
		digest := featureHash(canonical)
		body, _ := json.Marshal(map[string]any{"kind": "feature_partition", "job_id": j.ID, "partition": index, "feature_id": f.ID, "version": f.Version, "source_ref_id": j.Request.SourceRefID, "definition_digest": j.DefinitionDigest, "source_digest": j.SourceDigest, "values": values, "count": len(values), "embedding": def.Embedding})
		parents := []l.RefID{ref.RefID}
		if f.SourceRefID != j.AnchorRefID {
			anchor, _ := l.ParseRefID(j.AnchorRefID)
			parents = append(parents, anchor)
		}
		output, e := s.Assets.ImportDerived(ctx, p, l.Result, fmt.Sprintf("feature-job:%s:%d", j.ID, index), parents, body, ref.Policy)
		if e != nil {
			return fail(e)
		}
		if e = s.authorizeFeatureJob(ctx, p, j); e != nil {
			return fail(e)
		}
		part := d.FeaturePartition{Index: index, Count: len(values), Digest: digest, OutputRefID: output.RefID.String()}
		if e = r.CommitFeaturePartition(ctx, j, part, values); e != nil {
			return fail(e)
		}
	}
	// Derive the final watermark from immutable input, including partitions that
	// committed before restart; availability remains each partition's DB clock.
	if def.EventTimeField != "" {
		for col, name := range input.Columns {
			if name == def.EventTimeField {
				for _, row := range input.Rows {
					at, err := featureEvent(row[col])
					if err != nil {
						return fail(err)
					}
					if featureJobInclude(j, at) && (watermark == nil || at.After(*watermark)) {
						x := at
						watermark = &x
					}
				}
				break
			}
		}
	}
	return r.FinishFeatureJob(ctx, j, "completed", "", watermark)
}
func (s *Service) ValidateFeatureJobOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	if p.Validate() != nil || ref.TenantID != p.TenantID || ref.Kind != l.Result || !strings.HasPrefix(ref.ProducerID, "feature-job:") {
		return l.ErrForbidden
	}
	parts := strings.Split(strings.TrimPrefix(ref.ProducerID, "feature-job:"), ":")
	if len(parts) != 2 {
		return l.ErrForbidden
	}
	j, e := s.GetFeatureJob(ctx, p, parts[0])
	if e != nil || j.Status == "withdrawn" {
		return l.ErrForbidden
	}
	expected := map[string]bool{j.Request.SourceRefID: true, j.AnchorRefID: true}
	if len(ref.Parents) != len(expected) {
		return l.ErrForbidden
	}
	for _, parent := range ref.Parents {
		if !expected[parent.String()] {
			return l.ErrForbidden
		}
	}
	for _, part := range j.Partitions {
		if fmt.Sprint(part.Index) == parts[1] && part.Status == "committed" && part.OutputRefID == ref.RefID.String() {
			return nil
		}
	}
	return l.ErrForbidden
}
