//go:build integration

package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func wineBenchmarkExpression() d.FeatureDefinition {
	column := func(name string) d.FeatureExpression { return d.FeatureExpression{Op: "column", Field: name} }
	return d.FeatureDefinition{Kind: "expression", EntityField: "benchmark_row_id", EventTimeField: "benchmark_event_time", Expression: &d.FeatureExpression{Op: "add", Args: []d.FeatureExpression{
		{Op: "mul", Args: []d.FeatureExpression{{Op: "add", Args: []d.FeatureExpression{column("fixed_acidity"), column("volatile_acidity")}}, column("density")}},
		{Op: "literal", Value: json.RawMessage(`0.000000000000000001`)},
	}}}
}
func wineBenchmarkExpressionOracle(t *testing.T, row wineBenchmarkRow) *big.Rat {
	t.Helper()
	sum := new(big.Rat).Add(benchmarkRat(t, row.Values[0]), benchmarkRat(t, row.Values[1]))
	product := new(big.Rat).Mul(sum, benchmarkRat(t, row.Values[7]))
	return product.Add(product, benchmarkRat(t, "0.000000000000000001"))
}
func benchmarkRowIndex(t *testing.T, value d.FeatureValue) int {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimPrefix(value.EntityID, "row-"))
	require.NoError(t, err)
	return n
}
func benchmarkCheckExpression(t *testing.T, values []d.FeatureValue, rows []wineBenchmarkRow) {
	t.Helper()
	seen := map[int]bool{}
	for _, value := range values {
		index := benchmarkRowIndex(t, value)
		require.GreaterOrEqual(t, index, 0)
		require.Less(t, index, len(rows))
		require.False(t, seen[index])
		seen[index] = true
		assertBenchmarkValue(t, value, wineBenchmarkExpressionOracle(t, rows[index]))
		require.True(t, wineBenchmarkEvent(index).Equal(value.Timestamp))
	}
}
func benchmarkEntities(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("row-%05d", i)
	}
	return ids
}

func TestFeatureBenchmarkHFExactMaterializationBackfillAndAsOf(t *testing.T) {
	data := loadWineBenchmark(t)
	f := featureOperationsDB(t)
	ctx := context.Background()
	evidence := map[string]any{"dataset": data.Manifest, "protocol": "wine-feature-oracle-v1", "synthetic_metadata": "benchmark_row_id, benchmark_entity and minute event timestamps are test harness fields, not natural wine entities/events", "oracle": "independent rational arithmetic; all persisted numeric values checked via value_exact", "limits": map[string]int{"snapshot_rows": 10000, "cells": 100000, "direct_materialize_rows": 1000, "partition_rows": 1000, "query_entity_feature_pairs": 1000}, "production_sla": false}
	stages := []map[string]any{}
	defer func() {
		evidence["stages"] = stages
		evidence["passed"] = !t.Failed()
		benchmarkWriteReport(t, "feature-benchmark", evidence)
	}()

	small := benchmarkImport(t, f, wineBenchmarkInput(data.Rows[:1000], true), "original first 1000")
	feature := benchmarkCreateFeature(t, f, small, "wine exact arithmetic", wineBenchmarkExpression())
	started := time.Now()
	code, out := f.api(t, f.p, "POST", "/features/preview", "", map[string]any{"feature": feature, "limit": 100})
	require.Equal(t, 200, code, out)
	preview := benchmarkDecode[d.FeaturePreview](t, out["data"])
	require.Equal(t, 1000, preview.TotalCount)
	require.Len(t, preview.Values, 100)
	benchmarkCheckExpression(t, preview.Values, data.Rows)
	stages = append(stages, map[string]any{"name": "preview", "computed_rows": 1000, "displayed_rows": 100, "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000})
	key := uuid.NewString()
	started = time.Now()
	code, out = f.api(t, f.p, "POST", "/features/"+feature.ID+"/materialize", key, map[string]any{"version": 1})
	require.Equal(t, 200, code, out)
	receipt := benchmarkDecode[d.FeatureMaterialization](t, out["data"])
	require.Equal(t, 1000, receipt.Count)
	before := benchmarkQuery(t, f, feature, []string{"row-00000"}, receipt.AvailableAt.Add(-time.Microsecond))
	require.Empty(t, before)
	values := benchmarkQuery(t, f, feature, benchmarkEntities(1000), receipt.AvailableAt.Add(time.Microsecond))
	require.Len(t, values, 1000)
	benchmarkCheckExpression(t, values, data.Rows)
	code, out = f.api(t, f.p, "POST", "/features/"+feature.ID+"/materialize", key, map[string]any{"version": 1})
	require.Equal(t, 200, code, out)
	require.Equal(t, receipt.ReceiptID, benchmarkDecode[d.FeatureMaterialization](t, out["data"]).ReceiptID)
	stages = append(stages, map[string]any{"name": "synchronous_materialization_and_http_query", "rows": 1000, "exact_values_checked": 1000, "receipt_id": receipt.ReceiptID, "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000, "as_of_no_future_leak": true, "idempotent_receipt": true})

	baseRef := benchmarkImport(t, f, wineBenchmarkInput(data.Rows[:5000], true), "first 5000")
	durableFeature := benchmarkCreateFeature(t, f, baseRef, "wine durable exact arithmetic", wineBenchmarkExpression())
	full, duration := benchmarkRunFeatureJob(t, f, d.FeatureJobRequest{FeatureID: durableFeature.ID, FeatureVersion: 1, SourceRefID: baseRef.RefID.String(), Mode: "full", PartitionSize: 1000})
	require.Equal(t, 5000, full.CommittedRows)
	benchmarkCheckExpression(t, benchmarkJobValues(t, f, full), data.Rows)
	stages = append(stages, map[string]any{"name": "full", "rows": 5000, "partitions": len(full.Partitions), "elapsed_ms": float64(duration.Microseconds()) / 1000, "job_id": full.ID, "exact_values_checked": 5000})
	allRef := benchmarkImport(t, f, wineBenchmarkInput(data.Rows, true), "original complete 6497")
	inc, duration := benchmarkRunFeatureJob(t, f, d.FeatureJobRequest{FeatureID: durableFeature.ID, FeatureVersion: 1, SourceRefID: allRef.RefID.String(), Mode: "incremental", PartitionSize: 1000})
	require.Equal(t, 1497, inc.CommittedRows)
	require.True(t, inc.WatermarkBefore.Equal(*full.WatermarkAfter))
	require.True(t, inc.WatermarkAfter.Equal(wineBenchmarkEvent(6496)))
	benchmarkCheckExpression(t, benchmarkJobValues(t, f, inc), data.Rows)
	started = time.Now()
	values = benchmarkQuery(t, f, durableFeature, benchmarkEntities(6497), time.Now().Add(time.Second))
	require.Len(t, values, 6497)
	benchmarkCheckExpression(t, values, data.Rows)
	stages = append(stages, map[string]any{"name": "incremental", "snapshot_rows": 6497, "new_rows": 1497, "partitions": len(inc.Partitions), "execution_ms": float64(duration.Microseconds()) / 1000, "http_query_all_ms": float64(time.Since(started).Microseconds()) / 1000, "exact_values_checked": 6497, "job_id": inc.ID})
	// Explicitly synthetic correction: change fixed acidity for exactly 32 known
	// records, retaining the true raw dataset and original event coordinates.
	corrected := append([]wineBenchmarkRow(nil), data.Rows...)
	for i := 0; i < 32; i++ {
		corrected[i].Values = append([]string(nil), corrected[i].Values...)
		corrected[i].Values[0] = new(big.Rat).Add(benchmarkRat(t, corrected[i].Values[0]), big.NewRat(1, 10)).FloatString(3)
	}
	correctedRef := benchmarkImport(t, f, wineBenchmarkInput(corrected, true), "synthetic correction of fixed_acidity +0.1 on first32")
	from, to := wineBenchmarkEvent(0), wineBenchmarkEvent(32)
	back, duration := benchmarkRunFeatureJob(t, f, d.FeatureJobRequest{FeatureID: durableFeature.ID, FeatureVersion: 1, SourceRefID: correctedRef.RefID.String(), Mode: "backfill", PartitionSize: 1000, EventFrom: &from, EventTo: &to})
	require.Equal(t, 32, back.CommittedRows)
	require.True(t, back.WatermarkAfter.Equal(*inc.WatermarkAfter))
	benchmarkCheckExpression(t, benchmarkJobValues(t, f, back), corrected)
	available := *back.Partitions[0].AvailableAt
	values = benchmarkQuery(t, f, durableFeature, benchmarkEntities(32), available.Add(-time.Microsecond))
	require.Len(t, values, 32)
	benchmarkCheckExpression(t, values, data.Rows)
	values = benchmarkQuery(t, f, durableFeature, benchmarkEntities(32), available)
	require.Len(t, values, 32)
	benchmarkCheckExpression(t, values, corrected)
	code, out = operationsAPI(t, f, f.p, "POST", "/feature-jobs/"+back.ID+"/withdraw", uuid.NewString(), map[string]any{"expected_version": back.Version})
	require.Equal(t, 200, code, out)
	values = benchmarkQuery(t, f, durableFeature, benchmarkEntities(32), time.Now().Add(time.Second))
	require.Len(t, values, 32)
	benchmarkCheckExpression(t, values, data.Rows)
	values = benchmarkQuery(t, f, durableFeature, benchmarkEntities(32), available)
	require.Len(t, values, 32)
	benchmarkCheckExpression(t, values, corrected)
	stages = append(stages, map[string]any{"name": "backfill_and_withdraw", "synthetic_correction": true, "rows": 32, "elapsed_ms": float64(duration.Microseconds()) / 1000, "watermark_preserved": true, "before_during_after_history_verified": true, "job_id": back.ID})

	// Full original data uses 97,455 cells. 10k uses an explicit projection and
	// repeated rows, otherwise the production 100k-cell bound would be exceeded.
	expanded := make([]wineBenchmarkRow, 10000)
	for i := range expanded {
		expanded[i] = data.Rows[i%len(data.Rows)]
	}
	large := wineBenchmarkInput(expanded, true)
	keep := []int{0, 1, 7, 10, 12, 13, 14}
	projected := l.Input{Kind: "table", ColumnTypes: map[string]string{}}
	for _, col := range keep {
		name := large.Columns[col]
		projected.Columns = append(projected.Columns, name)
		projected.ColumnTypes[name] = large.ColumnTypes[name]
	}
	for _, row := range large.Rows {
		copy := make([]any, len(keep))
		for i, col := range keep {
			copy[i] = row[col]
		}
		projected.Rows = append(projected.Rows, copy)
	}
	largeRef := benchmarkImport(t, f, projected, "10000 explicitly repeated/projected workload")
	largeFeature := benchmarkCreateFeature(t, f, largeRef, "wine 10k boundary", wineBenchmarkExpression())
	code, out = f.api(t, f.p, "POST", "/features/preview", "", map[string]any{"feature": largeFeature, "limit": 100})
	require.Equal(t, 400, code, out)
	largeJob, duration := benchmarkRunFeatureJob(t, f, d.FeatureJobRequest{FeatureID: largeFeature.ID, FeatureVersion: 1, SourceRefID: largeRef.RefID.String(), Mode: "full", PartitionSize: 1000})
	require.Equal(t, 10000, largeJob.CommittedRows)
	require.Len(t, largeJob.Partitions, 10)
	benchmarkCheckExpression(t, benchmarkJobValues(t, f, largeJob), expanded)
	tooLarge := projected
	tooLarge.Rows = append(append([][]any(nil), projected.Rows...), append([]any(nil), projected.Rows[0]...))
	_, err := f.assets.ImportWithKey(ctx, f.p, tooLarge, "10001 rejected", uuid.NewString())
	require.Error(t, err)
	stages = append(stages, map[string]any{"name": "10k_boundary", "rows": 10000, "original_distinct_rows": 6497, "synthetic_replication": true, "projected_columns": projected.Columns, "partitions": 10, "elapsed_ms": float64(duration.Microseconds()) / 1000, "exact_values_checked": 10000, "preview_rejected": true, "10001_rejected": true, "job_id": largeJob.ID})
	// The fixture explicitly revokes current source authorization, not a real
	// external provider account. Committed receipts remain facts but cannot grant
	// permission to read values or launch another materialization after revocation.
	f.revoked.Store(true)
	code, out = f.api(t, f.p, "POST", "/feature-values/query", "", map[string]any{"entity_ids": []string{"row-00000"}, "features": []d.FeatureValue{{FeatureID: feature.ID, Version: 1}}, "as_of": time.Now().Add(time.Second)})
	require.Equal(t, 403, code, out)
	code, out = f.api(t, f.p, "POST", "/features/"+feature.ID+"/materialize", uuid.NewString(), map[string]any{"version": 1})
	require.Equal(t, 403, code, out)
	code, out = operationsAPI(t, f, f.p, "GET", "/feature-jobs/"+largeJob.ID, "", nil)
	require.Equal(t, 403, code, out)
	outputID, err := l.ParseRefID(receipt.OutputRefID)
	require.NoError(t, err)
	_, _, err = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, outputID)
	require.ErrorIs(t, err, l.ErrForbidden)
	storedReceipt, err := f.s.LookupFeatureMaterialization(ctx, f.p, key)
	require.NoError(t, err)
	require.NotNil(t, storedReceipt)
	require.Equal(t, receipt.ReceiptID, storedReceipt.ReceiptID)
	stages = append(stages, map[string]any{"name": "source_authorization_revoked", "fixture_authorizer": true, "value_query_http": 403, "materialize_http": 403, "job_read_http": 403, "derived_output_read_denied": true, "committed_receipt_preserved": true})
}

func TestFeatureBenchmarkHFWindowIndependentOracle(t *testing.T) {
	data := loadWineBenchmark(t)
	f := featureOperationsDB(t)
	// Four entities and uneven event ordering deliberately exercise sorting,
	// equal timestamps and the excluded lower/open boundary on real alcohol data.
	input := wineBenchmarkInput(data.Rows[:240], true)
	for i, row := range input.Rows {
		row[13] = fmt.Sprintf("window-entity-%d", i%4)
		row[14] = wineBenchmarkEvent((i / 4) * 5).Format(time.RFC3339Nano)
	}
	// Duplicate the first event for one entity using a different real sample.
	input.Rows[4][14] = input.Rows[0][14]
	for i, j := 0, len(input.Rows)-1; i < j; i, j = i+1, j-1 {
		input.Rows[i], input.Rows[j] = input.Rows[j], input.Rows[i]
	}
	ref := benchmarkImport(t, f, input, "window oracle original alcohol plus synthetic event metadata")
	report := map[string]any{"dataset": data.Manifest, "protocol": "wine-window-oracle-v1", "rows": 240, "synthetic_metadata": true, "window_seconds": 600, "boundary": "(t-600 seconds,t]", "production_sla": false}
	results := []map[string]any{}
	defer func() {
		report["results"] = results
		report["passed"] = !t.Failed()
		benchmarkWriteReport(t, "feature-window-benchmark", report)
	}()
	for _, aggregate := range []string{"sum", "mean", "min", "max", "count"} {
		def := d.FeatureDefinition{Kind: "window_aggregate", Field: "alcohol", EntityField: "benchmark_entity", EventTimeField: "benchmark_event_time", WindowSeconds: 600, Aggregate: aggregate}
		feature := benchmarkCreateFeature(t, f, ref, "wine window "+aggregate, def)
		started := time.Now()
		code, out := f.api(t, f.p, "POST", "/features/"+feature.ID+"/materialize", uuid.NewString(), map[string]any{"version": 1})
		require.Equal(t, 200, code, out)
		receipt := benchmarkDecode[d.FeatureMaterialization](t, out["data"])
		id, err := l.ParseRefID(receipt.OutputRefID)
		require.NoError(t, err)
		_, body, err := f.assets.Resolve(context.Background(), f.p.TenantID, f.p.UserID, id)
		require.NoError(t, err)
		var output struct {
			Values []d.FeatureValue `json:"values"`
		}
		require.NoError(t, json.Unmarshal(body, &output))
		require.Len(t, output.Values, 239)
		expectedKeys := map[string]bool{}
		for _, row := range input.Rows {
			expectedKeys[row[13].(string)+"/"+row[14].(string)] = true
		}
		require.Len(t, expectedKeys, 239)
		seen := map[string]bool{}
		for _, value := range output.Values {
			key := value.EntityID + "/" + value.Timestamp.UTC().Format(time.RFC3339Nano)
			require.True(t, expectedKeys[key], "unexpected entity/event window")
			require.False(t, seen[key], "duplicate entity/event window")
			seen[key] = true
			selected := []*big.Rat{}
			for _, row := range input.Rows {
				at, err := time.Parse(time.RFC3339Nano, row[14].(string))
				require.NoError(t, err)
				if row[13] == value.EntityID && at.After(value.Timestamp.Add(-600*time.Second)) && !at.After(value.Timestamp) {
					selected = append(selected, benchmarkRat(t, string(row[10].(json.Number))))
				}
			}
			require.NotEmpty(t, selected)
			want := new(big.Rat)
			for _, v := range selected {
				want.Add(want, v)
			}
			switch aggregate {
			case "mean":
				want.Quo(want, big.NewRat(int64(len(selected)), 1))
				want = benchmarkRoundedRat(want, 18)
			case "min":
				want.Set(selected[0])
				for _, v := range selected {
					if v.Cmp(want) < 0 {
						want.Set(v)
					}
				}
			case "max":
				want.Set(selected[0])
				for _, v := range selected {
					if v.Cmp(want) > 0 {
						want.Set(v)
					}
				}
			case "count":
				want.SetInt64(int64(len(selected)))
			}
			assertBenchmarkValue(t, value, want)
		}
		require.Equal(t, expectedKeys, seen, "all expected windows must be returned exactly once")
		results = append(results, map[string]any{"aggregate": aggregate, "exact_values_checked": len(output.Values), "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000, "receipt_id": receipt.ReceiptID})
	}
}
