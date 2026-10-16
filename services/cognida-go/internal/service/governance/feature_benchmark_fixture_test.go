//go:build integration

package governance_test

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The benchmark never downloads data or starts a service. The controller must
// provide immutable public files and an independently migrated review database.
const wineBenchmarkRevision = "a90690dd91e947252ac664b495fb043ca545da23"
const wineBenchmarkSeed = int64(20261016)

var wineBenchmarkColumns = []string{"fixed_acidity", "volatile_acidity", "citric_acid", "residual_sugar", "chlorides", "free_sulfur_dioxide", "total_sulfur_dioxide", "density", "ph", "sulphates", "alcohol", "quality"}

type wineBenchmarkFile struct {
	Split     string `json:"split"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Format    string `json:"format"`
	Delimiter string `json:"delimiter"`
	Rows      int    `json:"rows"`
}
type wineBenchmarkManifest struct {
	DatasetID string              `json:"dataset_id"`
	Revision  string              `json:"revision"`
	License   string              `json:"license"`
	Files     []wineBenchmarkFile `json:"files"`
}
type wineBenchmarkRow struct {
	Split  string
	Index  int
	Values []string
}
type wineBenchmarkDataset struct {
	Manifest   wineBenchmarkManifest
	Rows       []wineBenchmarkRow
	Red, White []wineBenchmarkRow
}

func loadWineBenchmark(t *testing.T) wineBenchmarkDataset {
	t.Helper()
	manifestPath := os.Getenv("COGNIDA_FEATURE_BENCHMARK_MANIFEST")
	if manifestPath == "" {
		t.Skip("COGNIDA_FEATURE_BENCHMARK_MANIFEST is required for the pinned real-data benchmark")
	}
	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var data wineBenchmarkDataset
	require.NoError(t, json.Unmarshal(raw, &data.Manifest))
	require.Equal(t, "codesignal/wine-quality", data.Manifest.DatasetID)
	require.Equal(t, wineBenchmarkRevision, data.Manifest.Revision)
	require.Equal(t, "cc-by-4.0", strings.ToLower(data.Manifest.License))
	require.Len(t, data.Manifest.Files, 2)
	seen := map[string]bool{}
	for _, file := range data.Manifest.Files {
		require.Contains(t, []string{"red", "white"}, file.Split)
		require.False(t, seen[file.Split])
		seen[file.Split] = true
		require.Equal(t, "csv", file.Format)
		require.Equal(t, ";", file.Delimiter)
		require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), file.SHA256)
		path := file.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(manifestPath), path)
		}
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		hash := sha256.Sum256(content)
		require.Equal(t, file.SHA256, hex.EncodeToString(hash[:]), "immutable dataset bytes changed")
		reader := csv.NewReader(strings.NewReader(string(content)))
		reader.Comma = ';'
		records, err := reader.ReadAll()
		require.NoError(t, err)
		require.Equal(t, file.Rows, len(records)-1)
		expectedRows := 1599
		if file.Split == "white" {
			expectedRows = 4898
		}
		require.Equal(t, expectedRows, file.Rows)
		headers := make([]string, len(records[0]))
		for i, h := range records[0] {
			headers[i] = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(h)), " ", "_")
		}
		require.Equal(t, wineBenchmarkColumns, headers)
		for index, record := range records[1:] {
			require.Len(t, record, len(headers))
			for _, text := range record {
				require.True(t, json.Valid([]byte(text)), "original decimal is not a JSON number")
				_, ok := new(big.Rat).SetString(text)
				require.True(t, ok)
			}
			row := wineBenchmarkRow{Split: file.Split, Index: index, Values: append([]string(nil), record...)}
			if file.Split == "red" {
				data.Red = append(data.Red, row)
			} else {
				data.White = append(data.White, row)
			}
		}
	}
	data.Rows = append(append([]wineBenchmarkRow(nil), data.Red...), data.White...)
	require.Len(t, data.Rows, 6497)
	return data
}

func wineBenchmarkInput(rows []wineBenchmarkRow, metadata bool) l.Input {
	input := l.Input{Kind: "table", Columns: append([]string(nil), wineBenchmarkColumns...), ColumnTypes: map[string]string{}}
	for _, name := range wineBenchmarkColumns {
		input.ColumnTypes[name] = "DECIMAL"
	}
	if metadata {
		input.Columns = append(input.Columns, "benchmark_row_id", "benchmark_entity", "benchmark_event_time")
		input.ColumnTypes["benchmark_row_id"] = "VARCHAR"
		input.ColumnTypes["benchmark_entity"] = "VARCHAR"
		input.ColumnTypes["benchmark_event_time"] = "TIMESTAMP"
	}
	for i, source := range rows {
		row := make([]any, len(source.Values))
		for j, text := range source.Values {
			row[j] = json.Number(text)
		}
		if metadata {
			row = append(row, fmt.Sprintf("row-%05d", i), fmt.Sprintf("synthetic-entity-%02d", i%16), wineBenchmarkEvent(i).Format(time.RFC3339Nano))
		}
		input.Rows = append(input.Rows, row)
	}
	return input
}
func wineBenchmarkEvent(index int) time.Time {
	return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Minute)
}
func benchmarkRat(t *testing.T, text string) *big.Rat {
	t.Helper()
	v, ok := new(big.Rat).SetString(text)
	require.True(t, ok, "invalid oracle decimal %q", text)
	return v
}

// Finite decimal serialization preserves every original digit of the injected
// arithmetic controls. This is oracle code and does not call production helpers.
func benchmarkFiniteDecimal(t *testing.T, value *big.Rat) string {
	t.Helper()
	denominator := new(big.Int).Set(value.Denom())
	places := 0
	for _, factor := range []int64{2, 5} {
		count := 0
		for new(big.Int).Mod(denominator, big.NewInt(factor)).Sign() == 0 {
			denominator.Quo(denominator, big.NewInt(factor))
			count++
		}
		places = max(places, count)
	}
	require.Zero(t, denominator.Cmp(big.NewInt(1)), "oracle requires a finite decimal")
	return value.FloatString(places)
}

func TestFeatureBenchmarkOracleKnownDecimals(t *testing.T) {
	for _, text := range []string{"0", "-0.1234567890123456789", "9.56666666666667", "9007199254740993.000000000000000001"} {
		require.Equal(t, text, benchmarkFiniteDecimal(t, benchmarkRat(t, text)))
	}
	require.Equal(t, "9.66666666666667", benchmarkFiniteDecimal(t, new(big.Rat).Add(benchmarkRat(t, "9.56666666666667"), benchmarkRat(t, "0.1"))))
	require.Zero(t, benchmarkRoundedRat(big.NewRat(1, 6), 18).Cmp(benchmarkRat(t, "0.166666666666666667")))
	require.Zero(t, benchmarkRoundedRat(big.NewRat(-1, 6), 18).Cmp(benchmarkRat(t, "-0.166666666666666667")))
}

// A separate quotient/remainder oracle checks the published 18-place rounding
// contract for non-terminating means; it does not call production arithmetic.
func benchmarkRoundedRat(value *big.Rat, places int) *big.Rat {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	numerator := new(big.Int).Mul(value.Num(), scale)
	sign := numerator.Sign()
	numerator.Abs(numerator)
	q, remainder := new(big.Int), new(big.Int)
	q.QuoRem(numerator, value.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(value.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if sign < 0 {
		q.Neg(q)
	}
	return new(big.Rat).SetFrac(q, scale)
}
func assertBenchmarkValue(t *testing.T, value d.FeatureValue, want *big.Rat) {
	t.Helper()
	require.NotEmpty(t, value.ValueExact)
	actual := benchmarkRat(t, value.ValueExact)
	require.Zero(t, actual.Cmp(want), "entity=%s event=%s actual=%s oracle=%s", value.EntityID, value.Timestamp.UTC().Format(time.RFC3339Nano), value.ValueExact, want.RatString())
}
func benchmarkDecode[T any](t *testing.T, value any) T {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}
func benchmarkImport(t *testing.T, f *featureDBFixture, input l.Input, name string) l.DataAssetRef {
	t.Helper()
	ref, err := f.assets.ImportWithKey(context.Background(), f.p, input, "HF benchmark: "+name, uuid.NewString())
	require.NoError(t, err)
	return ref
}
func benchmarkCreateFeature(t *testing.T, f *featureDBFixture, ref l.DataAssetRef, name string, def d.FeatureDefinition) d.Feature {
	t.Helper()
	raw, err := json.Marshal(def)
	require.NoError(t, err)
	input := d.Feature{Name: name, Type: "numerical", SourceRefID: ref.RefID.String(), Definition: raw}
	code, out := f.api(t, f.p, "POST", "/features", uuid.NewString(), input)
	require.Equal(t, 200, code, out)
	record := benchmarkDecode[d.Record](t, out["data"])
	feature, err := f.s.GetFeatureVersion(context.Background(), f.p, record.ID, 1)
	require.NoError(t, err)
	return feature
}
func benchmarkQuery(t *testing.T, f *featureDBFixture, feature d.Feature, entities []string, at time.Time) []d.FeatureValue {
	t.Helper()
	var result []d.FeatureValue
	for start := 0; start < len(entities); start += 1000 {
		end := min(start+1000, len(entities))
		code, out := f.api(t, f.p, "POST", "/feature-values/query", "", map[string]any{"entity_ids": entities[start:end], "features": []d.FeatureValue{{FeatureID: feature.ID, Version: feature.Version}}, "as_of": at})
		require.Equal(t, 200, code, out)
		result = append(result, benchmarkDecode[[]d.FeatureValue](t, out["data"].(map[string]any)["items"])...)
	}
	return result
}
func benchmarkRunFeatureJob(t *testing.T, f *featureDBFixture, request d.FeatureJobRequest) (*d.FeatureJob, time.Duration) {
	t.Helper()
	started := time.Now()
	key := uuid.NewString()
	code, out := operationsAPI(t, f, f.p, "POST", "/feature-jobs", key, request)
	require.Equal(t, 200, code, out)
	created := benchmarkDecode[d.FeatureJob](t, out["data"])
	require.NoError(t, f.s.RunFeatureJob(context.Background(), f.p.TenantID, f.p.UserID, created.ID))
	code, out = operationsAPI(t, f, f.p, "GET", "/feature-jobs/"+created.ID, "", nil)
	require.Equal(t, 200, code, out)
	result := benchmarkDecode[d.FeatureJob](t, out["data"])
	require.Equal(t, "completed", result.Status, result.Error)
	require.Equal(t, request.FeatureID, result.Request.FeatureID)
	require.Equal(t, request.SourceRefID, result.Request.SourceRefID)
	return &result, time.Since(started)
}
func benchmarkJobValues(t *testing.T, f *featureDBFixture, job *d.FeatureJob) []d.FeatureValue {
	t.Helper()
	var all []d.FeatureValue
	for _, part := range job.Partitions {
		require.Equal(t, "committed", part.Status)
		require.NotNil(t, part.AvailableAt)
		id, err := l.ParseRefID(part.OutputRefID)
		require.NoError(t, err)
		_, body, err := f.assets.Resolve(context.Background(), f.p.TenantID, f.p.UserID, id)
		require.NoError(t, err)
		var output struct {
			JobID        string           `json:"job_id"`
			Count        int              `json:"count"`
			Values       []d.FeatureValue `json:"values"`
			SourceDigest string           `json:"source_digest"`
		}
		require.NoError(t, json.Unmarshal(body, &output))
		require.Equal(t, job.ID, output.JobID)
		require.Equal(t, part.Count, output.Count)
		require.Equal(t, job.SourceDigest, output.SourceDigest)
		require.Len(t, output.Values, part.Count)
		all = append(all, output.Values...)
	}
	require.Len(t, all, job.CommittedRows)
	return all
}
func benchmarkWriteReport(t *testing.T, name string, report any) {
	t.Helper()
	raw, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
	directory := os.Getenv("COGNIDA_FEATURE_BENCHMARK_OUTPUT_DIR")
	if directory == "" {
		directory = t.TempDir()
	}
	require.NoError(t, os.MkdirAll(directory, 0700))
	path := filepath.Join(directory, name+".json")
	require.NoError(t, os.WriteFile(path, append(raw, '\n'), 0600))
	t.Logf("benchmark evidence: %s", path)
}
