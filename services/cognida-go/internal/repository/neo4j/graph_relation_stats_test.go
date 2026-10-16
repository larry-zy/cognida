package neo4j

import (
	"context"
	"errors"
	"math"
	"testing"

	"cognida/internal/model/knowledge"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/require"
)

type relationStatsDriver struct {
	neo4j.DriverWithContext
	session *relationStatsSession
	config  neo4j.SessionConfig
}

func (d *relationStatsDriver) NewSession(_ context.Context, config neo4j.SessionConfig) neo4j.SessionWithContext {
	d.config = config
	return d.session
}

type relationStatsSession struct {
	neo4j.SessionWithContext
	result *relationStatsResult
	err    error
	query  string
	params map[string]any
	closed bool
}

func (s *relationStatsSession) Run(_ context.Context, query string, params map[string]any, _ ...func(*neo4j.TransactionConfig)) (neo4j.ResultWithContext, error) {
	s.query, s.params = query, params
	return s.result, s.err
}

func (s *relationStatsSession) Close(context.Context) error { s.closed = true; return nil }

type relationStatsResult struct {
	neo4j.ResultWithContext
	records    []*neo4j.Record
	position   int
	err        error
	consumeErr error
}

func (r *relationStatsResult) Next(context.Context) bool {
	r.position++
	return r.position <= len(r.records)
}
func (r *relationStatsResult) Record() *neo4j.Record { return r.records[r.position-1] }
func (r *relationStatsResult) Err() error            { return r.err }

func (r *relationStatsResult) Consume(context.Context) (neo4j.ResultSummary, error) {
	return nil, r.consumeErr
}

func relationStatsRecord(kind any, bucket any, chunks any, count any, strength any, weight any) *neo4j.Record {
	return &neo4j.Record{Keys: []string{"relationType", "strengthBucket", "chunkIDs", "relationCount", "strengthSum", "weightSum"}, Values: []any{kind, bucket, chunks, count, strength, weight}}
}

func TestGraphRelationStatsAggregatesStoredJSONAndNativeLists(t *testing.T) {
	session := &relationStatsSession{result: &relationStatsResult{records: []*neo4j.Record{
		relationStatsRecord("owns", int64(2), `["chunk,one","chunk\"two"]`, int64(2), 5.0, 4.0),
		relationStatsRecord("owns", int64(4), []any{"c3"}, int64(1), 4.75, int64(2)),
		relationStatsRecord("related", int64(0), "null", int64(1), int64(0), int64(0)),
	}}}
	driver := &relationStatsDriver{session: session}
	repo := &Neo4jRepository{driver: driver, dbName: "isolated-graph"}
	stats, err := repo.GetRelationStats(context.Background(), knowledge.NameSpace{TenantID: "41", KnowledgeBaseID: "quoted-'kb"})
	require.NoError(t, err)
	require.Equal(t, 4, stats.TotalRelations)
	require.Equal(t, 9.75/4, stats.AverageStrength)
	require.Equal(t, 1.5, stats.AverageWeight)
	require.Equal(t, 1.25, stats.AverageChunkCount)
	require.Equal(t, map[int]int{0: 1, 2: 2, 4: 1}, stats.StrengthDistribution)
	require.Equal(t, map[string]int{"owns": 3, "related": 1}, stats.TypeDistribution)
	require.Equal(t, map[string]any{"tenantId": "41", "kbId": "quoted-'kb"}, session.params)
	require.NotContains(t, session.query, "quoted-'kb")
	require.NotContains(t, session.query, "LIMIT")
	require.Equal(t, neo4j.AccessModeRead, driver.config.AccessMode)
	require.True(t, session.closed)
}

func TestGraphRelationStatsEmptyAndFailures(t *testing.T) {
	t.Run("empty is real zero with empty distributions", func(t *testing.T) {
		session := &relationStatsSession{result: &relationStatsResult{}}
		repo := &Neo4jRepository{driver: &relationStatsDriver{session: session}}
		stats, err := repo.GetRelationStats(context.Background(), knowledge.NameSpace{TenantID: "41", KnowledgeBaseID: "empty"})
		require.NoError(t, err)
		require.Equal(t, &knowledge.RelationStatistics{StrengthDistribution: map[int]int{}, TypeDistribution: map[string]int{}}, stats)
	})
	for _, tc := range []struct {
		name              string
		record            *neo4j.Record
		runErr, streamErr error
	}{
		{name: "query error", runErr: errors.New("query unavailable")},
		{name: "late stream error", record: relationStatsRecord("owns", int64(1), nil, int64(1), 1.0, 1.0), streamErr: errors.New("stream interrupted")},
		{name: "malformed JSON", record: relationStatsRecord("owns", int64(1), `["c1"`, int64(1), 1.0, 1.0)},
		{name: "null chunk element", record: relationStatsRecord("owns", int64(1), `[null]`, int64(1), 1.0, 1.0)},
		{name: "native nonstring chunk", record: relationStatsRecord("owns", int64(1), []any{int64(2)}, int64(1), 1.0, 1.0)},
		{name: "invalid type", record: relationStatsRecord(int64(3), int64(1), nil, int64(1), 1.0, 1.0)},
		{name: "invalid strength", record: relationStatsRecord("owns", nil, nil, int64(1), 1.0, 1.0)},
		{name: "invalid count", record: relationStatsRecord("owns", int64(1), nil, int64(-1), 1.0, 1.0)},
		{name: "invalid aggregate", record: relationStatsRecord("owns", int64(1), nil, int64(1), "not numeric", 1.0)},
		{name: "nonfinite aggregate", record: relationStatsRecord("owns", int64(1), nil, int64(1), 1.0, math.Inf(1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &relationStatsResult{err: tc.streamErr}
			if tc.record != nil {
				result.records = []*neo4j.Record{tc.record}
			}
			session := &relationStatsSession{result: result, err: tc.runErr}
			repo := &Neo4jRepository{driver: &relationStatsDriver{session: session}}
			stats, err := repo.GetRelationStats(context.Background(), knowledge.NameSpace{TenantID: "41", KnowledgeBaseID: "kb"})
			require.Error(t, err)
			require.Nil(t, stats, "errors must never return partial or fake zero statistics")
			require.True(t, session.closed)
		})
	}
	for _, ns := range []knowledge.NameSpace{{}, {TenantID: "0", KnowledgeBaseID: "kb"}, {TenantID: "bad", KnowledgeBaseID: "kb"}, {TenantID: "41", KnowledgeBaseID: " "}} {
		stats, err := (&Neo4jRepository{}).GetRelationStats(context.Background(), ns)
		require.Error(t, err)
		require.Nil(t, stats)
	}
}

func TestGraphRelationStatsSingleWriteUsesScopeAndRealReceipt(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		hasRecord            bool
		streamErr, commitErr error
		wantErr              bool
	}{
		{name: "persisted", hasRecord: true},
		{name: "missing nodes", wantErr: true},
		{name: "stream failure", streamErr: errors.New("stream failed"), wantErr: true},
		{name: "commit failure", hasRecord: true, commitErr: errors.New("commit failed"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &relationStatsResult{err: tc.streamErr, consumeErr: tc.commitErr}
			if tc.hasRecord {
				result.records = []*neo4j.Record{{Keys: []string{"r"}, Values: []any{nil}}}
			}
			session := &relationStatsSession{result: result}
			repo := &Neo4jRepository{driver: &relationStatsDriver{session: session}}
			relation, err := repo.AddRelation(context.Background(), knowledge.NameSpace{TenantID: "41", KnowledgeBaseID: "kb"}, &knowledge.GraphRelation{ID: "new", Source: "A", Target: "B", Type: "owns", Strength: 5, ChunkIDs: []string{"a", "b", "c"}})
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, relation)
			} else {
				require.NoError(t, err)
				require.InDelta(t, 3.95, relation.Weight, 1e-12)
			}
			require.Contains(t, session.query, "MERGE (s)-[r:RELATION {id: $id, tenant_id: $tenantId, kb_id: $kbId}]")
			require.Equal(t, "41", session.params["tenantId"])
			require.Equal(t, "kb", session.params["kbId"])
			require.True(t, session.closed)
		})
	}
}
