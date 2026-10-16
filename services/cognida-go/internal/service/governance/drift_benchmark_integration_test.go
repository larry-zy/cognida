//go:build integration

package governance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"testing"
	"time"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const driftBenchmarkThreshold = 0.2
const driftBenchmarkMinimum = 20
const driftBenchmarkSamplingSeed = "wine-drift-benchmark-20261016"

type driftBenchmarkCase struct {
	Name                 string        `json:"name"`
	Class                string        `json:"class"`
	Window               int           `json:"window"`
	Synthetic            bool          `json:"synthetic"`
	Shift                string        `json:"shift,omitempty"`
	ExpectedIntervention bool          `json:"known_intervention"`
	Detected             bool          `json:"detected"`
	Status               string        `json:"status"`
	ElapsedMS            float64       `json:"elapsed_ms"`
	ReportID             string        `json:"report_id"`
	Report               d.DriftReport `json:"report"`
}

func driftBenchmarkInput(rows []wineBenchmarkRow) l.Input {
	input := wineBenchmarkInput(rows, false)
	// An explicitly derived categorical field exercises total variation without
	// injecting row identifiers, synthetic event times or the group label.
	input.Columns = append(input.Columns, "derived_quality_band")
	input.ColumnTypes["derived_quality_band"] = "VARCHAR"
	for i, row := range rows {
		quality, _ := new(big.Rat).SetString(row.Values[11])
		band := "quality<=5"
		if quality.Cmp(big.NewRat(5, 1)) > 0 {
			band = "quality>5"
		}
		input.Rows[i] = append(input.Rows[i], band)
	}
	return input
}
func cloneDriftBenchmark(input l.Input) l.Input {
	out := input
	out.Columns = append([]string(nil), input.Columns...)
	out.ColumnTypes = map[string]string{}
	for k, v := range input.ColumnTypes {
		out.ColumnTypes[k] = v
	}
	out.Rows = make([][]any, len(input.Rows))
	for i, row := range input.Rows {
		out.Rows[i] = append([]any(nil), row...)
	}
	return out
}
func driftBenchmarkSample(input l.Input, limit int) [][]any {
	// Reproduce only the documented snapshot sampling contract independently;
	// statistical effects below use pooled frequencies, not production's KS scan.
	type ranked struct {
		index int
		hash  [32]byte
	}
	ranks := make([]ranked, len(input.Rows))
	for i := range ranks {
		ranks[i] = ranked{i, sha256.Sum256([]byte(fmt.Sprintf("%s:%d", driftBenchmarkSamplingSeed, i)))}
	}
	sort.Slice(ranks, func(i, j int) bool { return bytes.Compare(ranks[i].hash[:], ranks[j].hash[:]) < 0 })
	out := [][]any{}
	for _, r := range ranks[:min(limit, len(ranks))] {
		out = append(out, input.Rows[r.index])
	}
	return out
}
func driftBenchmarkKSOracle(t *testing.T, a, b [][]any, colA, colB int) (float64, int, int) {
	t.Helper()
	type point struct {
		value *big.Rat
		a, b  int
	}
	frequencies := map[string]*point{}
	n, m := 0, 0
	for side, rows := range [][][]any{a, b} {
		col := colA
		if side == 1 {
			col = colB
		}
		for _, row := range rows {
			if row[col] == nil {
				continue
			}
			value := benchmarkRat(t, fmt.Sprint(row[col]))
			key := value.RatString()
			if frequencies[key] == nil {
				frequencies[key] = &point{value: value}
			}
			if side == 0 {
				frequencies[key].a++
				n++
			} else {
				frequencies[key].b++
				m++
			}
		}
	}
	points := make([]*point, 0, len(frequencies))
	for _, point := range frequencies {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].value.Cmp(points[j].value) < 0 })
	if n == 0 || m == 0 {
		return 0, n, m
	}
	ca, cb, maxDifference := 0, 0, 0
	for _, point := range points {
		ca += point.a
		cb += point.b
		difference := ca*m - cb*n
		if difference < 0 {
			difference = -difference
		}
		if difference > maxDifference {
			maxDifference = difference
		}
	}
	return float64(maxDifference) / float64(n*m), n, m
}
func driftBenchmarkTVOracle(a, b [][]any, colA, colB int) (float64, int, int) {
	type count struct{ a, b int }
	counts := map[string]*count{}
	n, m := 0, 0
	for side, rows := range [][][]any{a, b} {
		col := colA
		if side == 1 {
			col = colB
		}
		for _, row := range rows {
			if row[col] == nil {
				continue
			}
			key := fmt.Sprintf("%T:%v", row[col], row[col])
			if counts[key] == nil {
				counts[key] = &count{}
			}
			if side == 0 {
				counts[key].a++
				n++
			} else {
				counts[key].b++
				m++
			}
		}
	}
	if n == 0 || m == 0 {
		return 0, n, m
	}
	difference := 0
	for _, v := range counts {
		delta := v.a*m - v.b*n
		if delta < 0 {
			delta = -delta
		}
		difference += delta
	}
	return float64(difference) / float64(2*n*m), n, m
}
func TestDriftBenchmarkOracleKnownDistributions(t *testing.T) {
	numbers := func(values ...int) [][]any {
		rows := make([][]any, len(values))
		for i, value := range values {
			rows[i] = []any{value}
		}
		return rows
	}
	distance, n, m := driftBenchmarkKSOracle(t, numbers(1, 2, 3, 4), numbers(3, 4, 5, 6), 0, 0)
	require.Equal(t, 0.5, distance)
	require.Equal(t, 4, n)
	require.Equal(t, 4, m)
	distance, n, m = driftBenchmarkTVOracle([][]any{{"a"}, {"a"}, {"b"}, {"b"}}, [][]any{{"a"}, {"c"}, {"c"}, {"c"}}, 0, 0)
	require.Equal(t, 0.75, distance)
	require.Equal(t, 4, n)
	require.Equal(t, 4, m)
}
func checkDriftBenchmarkOracle(t *testing.T, a, b l.Input, report d.DriftReport) {
	t.Helper()
	sa, sb := driftBenchmarkSample(a, 1000), driftBenchmarkSample(b, 1000)
	require.Equal(t, len(a.Rows), report.BaselineRows)
	require.Equal(t, len(b.Rows), report.CurrentRows)
	require.Equal(t, len(sa), report.BaselineSample)
	require.Equal(t, len(sb), report.CurrentSample)
	require.Equal(t, 1000, report.Sampling.Limit)
	require.Equal(t, 20, report.Sampling.Minimum)
	ca, cb := map[string]int{}, map[string]int{}
	for i, c := range a.Columns {
		ca[c] = i
	}
	for i, c := range b.Columns {
		cb[c] = i
	}
	expectedMetrics := map[string]bool{}
	changed := false
	for _, metric := range report.Metrics {
		key := metric.Field + "/" + metric.Method
		require.False(t, expectedMetrics[key])
		expectedMetrics[key] = true
		ia, oka := ca[metric.Field]
		ib, okb := cb[metric.Field]
		if metric.Method == "schema" {
			want := "stable"
			if !oka || !okb || a.ColumnTypes[metric.Field] != b.ColumnTypes[metric.Field] {
				want = "changed"
				changed = true
			}
			require.Equal(t, want, metric.Status)
			continue
		}
		require.True(t, oka && okb)
		require.Equal(t, driftBenchmarkThreshold, metric.Threshold)
		effect, n, m := 0.0, 0, 0
		switch metric.Method {
		case "missing_rate_delta":
			na, nb := 0, 0
			for _, row := range sa {
				if row[ia] == nil {
					na++
				}
			}
			for _, row := range sb {
				if row[ib] == nil {
					nb++
				}
			}
			n, m = len(sa), len(sb)
			effect = math.Abs(float64(na*m-nb*n)) / float64(n*m)
		case "empirical_ks":
			effect, n, m = driftBenchmarkKSOracle(t, sa, sb, ia, ib)
		case "categorical_total_variation":
			effect, n, m = driftBenchmarkTVOracle(sa, sb, ia, ib)
		default:
			t.Fatalf("unexpected statistical method %q", metric.Method)
		}
		require.Equal(t, n, metric.BaselineCount)
		require.Equal(t, m, metric.CurrentCount)
		if n < driftBenchmarkMinimum || m < driftBenchmarkMinimum {
			require.Equal(t, "insufficient", metric.Status)
			require.Nil(t, metric.Effect)
			continue
		}
		require.NotNil(t, metric.Effect)
		require.InDelta(t, effect, *metric.Effect, 1e-12, key)
		want := "stable"
		if effect >= driftBenchmarkThreshold {
			want = "changed"
			changed = true
		}
		require.Equal(t, want, metric.Status, key)
	}
	fields := map[string]bool{}
	for name := range ca {
		fields[name] = true
	}
	for name := range cb {
		fields[name] = true
	}
	for name := range fields {
		require.True(t, expectedMetrics[name+"/schema"], name)
		_, oka := ca[name]
		_, okb := cb[name]
		if !oka || !okb {
			continue
		}
		require.True(t, expectedMetrics[name+"/missing_rate_delta"], name)
		if a.ColumnTypes[name] != b.ColumnTypes[name] {
			continue
		}
		method := "empirical_ks"
		if a.ColumnTypes[name] == "VARCHAR" {
			method = "categorical_total_variation"
		}
		require.True(t, expectedMetrics[name+"/"+method], name)
	}
	require.Equal(t, changed, report.Changed)
}

func TestDriftBenchmarkHFStableGroupsAndSyntheticInterventions(t *testing.T) {
	data := loadWineBenchmark(t)
	f := featureOperationsDB(t)
	red, white := append([]wineBenchmarkRow(nil), data.Red...), append([]wineBenchmarkRow(nil), data.White...)
	rng := rand.New(rand.NewSource(wineBenchmarkSeed))
	rng.Shuffle(len(red), func(i, j int) { red[i], red[j] = red[j], red[i] })
	rng.Shuffle(len(white), func(i, j int) { white[i], white[j] = white[j], white[i] })
	baselineRows := append(append([]wineBenchmarkRow(nil), red[:250]...), white[:750]...)
	baselineInput := driftBenchmarkInput(baselineRows)
	evidence := map[string]any{"dataset": data.Manifest, "protocol": "wine-drift-v1-fixed-before-observation", "seed": wineBenchmarkSeed, "threshold": driftBenchmarkThreshold, "minimum": driftBenchmarkMinimum, "sampling_limit": 1000, "stable_windows": 20, "stable_window_rows": 200, "baseline_rows": 1000, "baseline_mix": "250 red + 750 white; stable current windows each 50 red + 150 white, disjoint from baseline and one another", "limitations": []string{"No natural business drift event gold labels exist; red/white is a cross-sectional group comparison, not a detection-rate benchmark.", "False alarms are observed flags on predeclared stratified controls, not a calibrated population significance level.", "Synthetic detection metrics describe the listed injected shifts only; distribution change does not establish quality or model-performance loss.", "Synthetic delay is counted in observation windows, not real time.", "Wall-clock durations include local HTTP handler, object I/O and MySQL operations and are not a production SLA."}}
	cases := []driftBenchmarkCase{}
	defer func() {
		evidence["cases"] = cases
		evidence["passed"] = !t.Failed()
		benchmarkWriteReport(t, "drift-benchmark", evidence)
	}()
	createBaseline := func(name string, input l.Input) string {
		source := benchmarkImport(t, f, input, name)
		code, out := operationsAPI(t, f, f.p, "POST", "/drift/baselines", uuid.NewString(), d.DriftBaseline{Name: name, SourceRefID: source.RefID.String(), Sampling: d.DriftSampling{Limit: 1000, Minimum: 20, Seed: driftBenchmarkSamplingSeed}})
		require.Equal(t, 200, code, out)
		record := benchmarkDecode[d.DriftRecord](t, out["data"])
		code, out = operationsAPI(t, f, f.p, "POST", "/drift/baselines/"+record.ID+"/approve", uuid.NewString(), map[string]any{"expected_version": record.Version})
		require.Equal(t, 200, code, out)
		return record.ID
	}
	baselineID := createBaseline("wine stratified reference", baselineInput)
	run := func(baseID string, base, current l.Input, name, class string, window int, synthetic, intervention bool, shift string) driftBenchmarkCase {
		started := time.Now()
		source := benchmarkImport(t, f, current, name)
		code, out := operationsAPI(t, f, f.p, "POST", "/drift/compare", "", map[string]any{"baseline_id": baseID, "current_ref_id": source.RefID.String(), "threshold": driftBenchmarkThreshold})
		require.Equal(t, 200, code, out)
		record := benchmarkDecode[d.DriftRecord](t, out["data"])
		var report d.DriftReport
		require.NoError(t, json.Unmarshal(record.Body, &report))
		require.Equal(t, baseID, report.BaselineID)
		require.Equal(t, source.RefID.String(), report.CurrentRefID)
		checkDriftBenchmarkOracle(t, base, current, report)
		result := driftBenchmarkCase{Name: name, Class: class, Window: window, Synthetic: synthetic, Shift: shift, ExpectedIntervention: intervention, Detected: report.Changed, Status: record.Status, ElapsedMS: float64(time.Since(started).Microseconds()) / 1000, ReportID: record.ID, Report: report}
		cases = append(cases, result)
		return result
	}
	identical := run(baselineID, baselineInput, baselineInput, "identical", "deterministic_control", 0, false, false, "")
	require.False(t, identical.Detected)
	permuted := cloneDriftBenchmark(baselineInput)
	rng.Shuffle(len(permuted.Rows), func(i, j int) { permuted.Rows[i], permuted.Rows[j] = permuted.Rows[j], permuted.Rows[i] })
	require.False(t, run(baselineID, baselineInput, permuted, "permutation", "deterministic_control", 1, false, false, "").Detected)
	falseAlarms := 0
	for window := 0; window < 20; window++ {
		rows := append(append([]wineBenchmarkRow(nil), red[250+window*50:300+window*50]...), white[750+window*150:900+window*150]...)
		current := driftBenchmarkInput(rows)
		if run(baselineID, baselineInput, current, fmt.Sprintf("stable-stratified-%02d", window), "stable_control", window, false, false, "").Detected {
			falseAlarms++
		}
	}
	evidence["observed_false_alarm_count"] = falseAlarms
	evidence["observed_false_alarm_denominator"] = 20
	evidence["observed_false_alarm_rate"] = float64(falseAlarms) / 20
	redInput, whiteInput := driftBenchmarkInput(red[:1000]), driftBenchmarkInput(white[:1000])
	redBase := createBaseline("wine red reference", redInput)
	run(redBase, redInput, whiteInput, "red-versus-white", "real_group_comparison", 0, false, false, "")

	syntheticCount, detectedCount := 0, 0
	shifted := func(offset string) l.Input {
		current := cloneDriftBenchmark(baselineInput)
		for _, row := range current.Rows {
			value := benchmarkRat(t, fmt.Sprint(row[10]))
			value.Add(value, benchmarkRat(t, offset))
			row[10] = json.Number(benchmarkFiniteDecimal(t, value))
		}
		return current
	}
	abrupt := run(baselineID, baselineInput, shifted("5"), "abrupt-alcohol-plus5", "synthetic_abrupt", 0, true, true, "+5 alcohol")
	require.True(t, abrupt.Detected)
	syntheticCount++
	if abrupt.Detected {
		detectedCount++
	}
	evidence["abrupt_first_detection_delay_windows"] = 0
	// A monitor with a fixed CurrentRefID revisits the same immutable snapshot.
	// Creating newer data does not turn that configuration into a live connector.
	code, out := operationsAPI(t, f, f.p, "POST", "/drift/monitors", uuid.NewString(), d.DriftMonitor{Name: "fixed HF snapshot monitor", BaselineID: baselineID, CurrentRefID: abrupt.Report.CurrentRefID, IntervalSeconds: 60, Threshold: driftBenchmarkThreshold})
	require.Equal(t, 200, code, out)
	monitorID := benchmarkDecode[d.DriftRecord](t, out["data"]).ID
	var monitoredReports []string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			benchmarkImport(t, f, shifted("10"), "newer snapshot deliberately not selected by fixed monitor")
		}
		monitor, err := f.s.GetDrift(context.Background(), f.p, monitorID)
		require.NoError(t, err)
		require.NoError(t, f.s.RunDriftMonitor(context.Background(), *monitor))
		monitor, err = f.s.GetDrift(context.Background(), f.p, monitorID)
		require.NoError(t, err)
		var config d.DriftMonitor
		require.NoError(t, json.Unmarshal(monitor.Body, &config))
		require.Empty(t, config.LastError)
		require.Nil(t, config.Refresh)
		require.Equal(t, abrupt.Report.CurrentRefID, config.CurrentRefID)
		require.Equal(t, abrupt.ReportID, config.LastReportID)
		monitoredReports = append(monitoredReports, config.LastReportID)
	}
	evidence["fixed_snapshot_monitor"] = map[string]any{"runs": 2, "report_ids": monitoredReports, "same_snapshot_reused": true, "newer_unselected_snapshot_ignored": true, "automatic_latest_production_batch_fetch": false, "note": "Only explicit refresh connector configuration can request a fresh snapshot; no live connector was configured or benchmarked."}
	for i := 0; i < 3; i++ {
		require.False(t, run(baselineID, baselineInput, baselineInput, fmt.Sprintf("gradual-prelude-%d", i), "synthetic_gradual", i, true, false, "0").Detected)
	}
	firstDetection := -1
	for i, offset := range []string{"0.1", "0.25", "0.5", "0.75", "1", "1.5", "2"} {
		result := run(baselineID, baselineInput, shifted(offset), "gradual-alcohol-plus"+offset, "synthetic_gradual", i+3, true, true, "+"+offset+" alcohol")
		syntheticCount++
		if result.Detected {
			detectedCount++
			if firstDetection < 0 {
				firstDetection = i
			}
		}
	}
	require.GreaterOrEqual(t, firstDetection, 0, "large prespecified endpoint shift must be detected")
	evidence["gradual_first_detection_delay_windows"] = firstDetection
	missing := cloneDriftBenchmark(baselineInput)
	for i := range missing.Rows {
		if i%2 == 0 {
			missing.Rows[i][10] = nil
		}
	}
	require.True(t, run(baselineID, baselineInput, missing, "missing-alcohol-50percent", "synthetic_missing", 0, true, true, "every second alcohol cell set null").Detected)
	syntheticCount++
	detectedCount++
	dropped := cloneDriftBenchmark(baselineInput)
	dropped.Columns = append(dropped.Columns[:7], dropped.Columns[8:]...)
	delete(dropped.ColumnTypes, "density")
	for i, row := range dropped.Rows {
		dropped.Rows[i] = append(row[:7], row[8:]...)
	}
	require.True(t, run(baselineID, baselineInput, dropped, "schema-drop-density", "synthetic_schema", 0, true, true, "density removed").Detected)
	syntheticCount++
	detectedCount++
	typed := cloneDriftBenchmark(baselineInput)
	typed.ColumnTypes["alcohol"] = "VARCHAR"
	for _, row := range typed.Rows {
		row[10] = fmt.Sprint(row[10])
	}
	require.True(t, run(baselineID, baselineInput, typed, "schema-alcohol-to-text", "synthetic_schema", 1, true, true, "alcohol DECIMAL to VARCHAR").Detected)
	syntheticCount++
	detectedCount++
	evidence["synthetic_detection_count"] = detectedCount
	evidence["synthetic_intervention_windows"] = syntheticCount
	evidence["synthetic_detection_rate"] = float64(detectedCount) / float64(syntheticCount)
	insufficient := cloneDriftBenchmark(baselineInput)
	insufficient.Rows = insufficient.Rows[:19]
	require.Equal(t, "insufficient", run(baselineID, baselineInput, insufficient, "insufficient-19", "boundary", 0, false, false, "").Status)
	// Explicit real-row projection stays within the 100k-cell input ceiling.
	expanded := cloneDriftBenchmark(baselineInput)
	expanded.Columns = expanded.Columns[:8]
	expanded.ColumnTypes = map[string]string{}
	for _, name := range expanded.Columns {
		expanded.ColumnTypes[name] = "DECIMAL"
	}
	expanded.Rows = nil
	for i := 0; i < 10000; i++ {
		expanded.Rows = append(expanded.Rows, append([]any(nil), baselineInput.Rows[i%1000][:8]...))
	}
	largeBase := createBaseline("wine 10k projected repeated reference", expanded)
	large := run(largeBase, expanded, expanded, "10k-identical-capped-sampling", "synthetic_scale_control", 0, true, false, "10x repeated 1000 real rows, 8 columns")
	require.False(t, large.Detected)
	require.Equal(t, 1000, large.Report.BaselineSample)
	require.Equal(t, 1000, large.Report.CurrentSample)
	t.Logf("drift stable control flags=%d/20; synthetic detections=%d/%d; gradual delay=%d windows; threshold fixed %.1f", falseAlarms, detectedCount, syntheticCount, firstDetection, driftBenchmarkThreshold)
}
