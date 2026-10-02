package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

func watchColumns(input l.Input, in d.AnalysisInput, watch *d.Watch) (int, int, []int, error) {
	if input.Kind != "table" || watch == nil || watch.Method != "zscore" || watch.Threshold <= 0 || watch.Threshold > 20 || math.IsNaN(watch.Threshold) || len(watch.AttributionDimensions) > 10 {
		return 0, 0, nil, d.ErrInvalid
	}
	value, at := -1, -1
	indexes := map[string]int{}
	for i, c := range input.Columns {
		indexes[c] = i
		if c == in.ValueColumn {
			value = i
		}
		if c == in.TimeColumn {
			at = i
		}
	}
	if value < 0 || (in.TimeColumn != "" && at < 0) {
		return 0, 0, nil, fmt.Errorf("%w: watch columns missing", d.ErrInvalid)
	}
	dims := []int{}
	seen := map[string]bool{}
	for _, dim := range watch.AttributionDimensions {
		idx, ok := indexes[dim]
		if !ok || seen[dim] || dim == in.ValueColumn || dim == in.TimeColumn {
			return 0, 0, nil, d.ErrInvalid
		}
		seen[dim] = true
		dims = append(dims, idx)
	}
	if len(dims) > 0 && at < 0 {
		return 0, 0, nil, fmt.Errorf("%w: attribution requires time_column", d.ErrInvalid)
	}
	return value, at, dims, nil
}
func WatchMetric(input l.Input, in d.AnalysisInput, watch *d.Watch) (map[string]any, bool, error) {
	if err := input.Validate(); err != nil {
		return nil, false, d.ErrInvalid
	}
	valueIndex, timeIndex, dims, err := watchColumns(input, in, watch)
	if err != nil {
		return nil, false, err
	}
	type observation struct {
		value  float64
		at     time.Time
		groups map[string]float64
	}
	values := []*observation{}
	times := map[time.Time]*observation{}
	labels := map[string]map[string]any{}
	for _, row := range input.Rows {
		if row[valueIndex] == nil {
			continue
		}
		v, err := strconv.ParseFloat(fmt.Sprint(row[valueIndex]), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false, fmt.Errorf("%w: nonnumeric watch value", d.ErrInvalid)
		}
		o := &observation{value: v, groups: map[string]float64{}}
		if timeIndex >= 0 {
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02", "2006-01-02 15:04:05"} {
				o.at, err = time.Parse(layout, fmt.Sprint(row[timeIndex]))
				if err == nil {
					break
				}
			}
			if err != nil {
				return nil, false, fmt.Errorf("%w: invalid time value", d.ErrInvalid)
			}
			o.at = o.at.UTC()
			if existing := times[o.at]; existing != nil {
				existing.value += v
				o = existing
			} else {
				times[o.at] = o
				values = append(values, o)
			}
		} else {
			values = append(values, o)
		}
		if len(dims) > 0 {
			parts := make([]any, 0, len(dims))
			group := map[string]any{}
			for j, idx := range dims {
				parts = append(parts, row[idx])
				group[watch.AttributionDimensions[j]] = row[idx]
			}
			raw, _ := json.Marshal(parts)
			key := string(raw)
			labels[key] = group
			o.groups[key] += v
		}
		if math.IsInf(o.value, 0) || math.IsNaN(o.value) {
			return nil, false, d.ErrInvalid
		}
	}
	if len(values) < 4 {
		return nil, false, fmt.Errorf("%w: watch requires four distinct time periods (or rows without time_column)", d.ErrInvalid)
	}
	if timeIndex >= 0 {
		sort.Slice(values, func(i, j int) bool { return values[i].at.Before(values[j].at) })
	}
	latest := values[len(values)-1]
	base := values[:len(values)-1]
	mean := 0.0
	for _, o := range base {
		mean += o.value / float64(len(base))
	}
	variance := 0.0
	for _, o := range base {
		delta := o.value - mean
		variance += delta * delta / float64(len(base))
	}
	if math.IsInf(variance, 0) || math.IsNaN(variance) {
		return nil, false, d.ErrInvalid
	}
	deviation := math.Sqrt(variance)
	var score any
	trigger := false
	if deviation == 0 {
		trigger = latest.value != mean
	} else {
		z := math.Abs(latest.value-mean) / deviation
		if math.IsInf(z, 0) {
			return nil, false, d.ErrInvalid
		}
		score = z
		trigger = z >= watch.Threshold
	}
	out := map[string]any{"method": "zscore", "aggregation": "sum_per_time_period", "latest": latest.value, "baseline_mean": mean, "baseline_stddev": deviation, "baseline_count": len(base), "zscore": score, "threshold": watch.Threshold, "triggered": trigger, "constant_baseline": deviation == 0}
	if timeIndex < 0 {
		out["aggregation"] = "row_order"
	} else {
		out["latest_period"] = latest.at
	}
	attribution := []map[string]any{}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		baseline := 0.0
		for _, o := range base {
			baseline += o.groups[key] / float64(len(base))
		}
		delta := latest.groups[key] - baseline
		var share any
		if latest.value != mean {
			share = delta / (latest.value - mean)
		}
		attribution = append(attribution, map[string]any{"dimensions": labels[key], "latest": latest.groups[key], "baseline_mean": baseline, "delta": delta, "share_of_total_delta": share})
	}
	sort.SliceStable(attribution, func(i, j int) bool {
		return math.Abs(attribution[i]["delta"].(float64)) > math.Abs(attribution[j]["delta"].(float64))
	})
	out["attribution"] = attribution
	out["attribution_method"] = "latest_minus_baseline_mean_by_dimension"
	out["attribution_assumptions"] = []string{"Metric is additive across rows within each time period", "Missing dimension groups contribute zero", "Contributions describe association, not causation"}
	return out, trigger, nil
}
