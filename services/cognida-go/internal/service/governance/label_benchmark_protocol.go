package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	d "cognida/internal/model/governance"
)

const LabelBenchmarkSeed = "cognida-hf-label-v1-20261016"

type LabelBenchmarkSelection struct {
	ID               string `json:"id"`
	TextSHA256       string `json:"text_sha256"`
	TrainTextOverlap bool   `json:"train_text_overlap"`
}

func LabelBenchmarkTextDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Fixed pre-registration selection. Classification is stratified at at most two rows per
// declared class; NER is 64 hash-ranked unseen texts. No predictions are inputs.
func SelectLabelBenchmarkModelSamples(kind string, labels []d.Label, train, test []LabelBenchmarkSample) ([]LabelBenchmarkSelection, error) {
	if ValidateLabelSchema(labels) != nil {
		return nil, d.ErrInvalid
	}
	seen := map[string]bool{}
	for _, row := range train {
		if row.Split != "train" {
			return nil, d.ErrInvalid
		}
		seen[LabelBenchmarkTextDigest(row.Text)] = true
	}
	groups := map[string][]LabelBenchmarkSample{}
	known := map[string]bool{}
	for _, label := range labels {
		known[label.ID] = true
	}
	ids := map[string]bool{}
	for _, row := range test {
		if row.Split != "test" || ids[row.ID] || row.ID == "" {
			return nil, d.ErrInvalid
		}
		ids[row.ID] = true
		if kind == "classification" {
			if !known[row.Label] {
				return nil, d.ErrInvalid
			}
			groups[row.Label] = append(groups[row.Label], row)
		} else if kind == "named_entity" {
			if !seen[LabelBenchmarkTextDigest(row.Text)] {
				groups["ner"] = append(groups["ner"], row)
			}
		} else {
			return nil, d.ErrInvalid
		}
	}
	keys := []string{"ner"}
	limit := 64
	if kind == "classification" {
		keys = nil
		limit = 2
		for _, label := range labels {
			keys = append(keys, label.ID)
		}
	}
	var selected []LabelBenchmarkSelection
	for _, key := range keys {
		rows := groups[key]
		take := limit
		if kind == "classification" && len(rows) < take {
			take = len(rows)
		}
		if kind == "named_entity" && len(rows) < take {
			return nil, fmt.Errorf("insufficient pre-registration samples for %s", key)
		}
		sort.Slice(rows, func(i, j int) bool {
			a := LabelBenchmarkTextDigest(LabelBenchmarkSeed + "\x00" + rows[i].ID)
			b := LabelBenchmarkTextDigest(LabelBenchmarkSeed + "\x00" + rows[j].ID)
			if a == b {
				return rows[i].ID < rows[j].ID
			}
			return a < b
		})
		for _, row := range rows[:take] {
			digest := LabelBenchmarkTextDigest(row.Text)
			selected = append(selected, LabelBenchmarkSelection{row.ID, digest, seen[digest]})
		}
	}
	return selected, nil
}
