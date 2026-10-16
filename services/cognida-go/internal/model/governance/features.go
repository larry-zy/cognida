package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// FeatureDefinition is a closed declarative language, never executable code.
// Windows are (event_time-window_seconds,event_time], independently per entity.
type FeatureDefinition struct {
	Kind           string             `json:"kind"`
	Field          string             `json:"field,omitempty"`
	EntityField    string             `json:"entity_field,omitempty"`
	EventTimeField string             `json:"event_time_field,omitempty"`
	Expression     *FeatureExpression `json:"expression,omitempty"`
	WindowSeconds  int64              `json:"window_seconds,omitempty"`
	Aggregate      string             `json:"aggregate,omitempty"`
	Embedding      *FeatureEmbedding  `json:"embedding,omitempty"`
}
type FeatureEmbedding struct {
	ModelID   string `json:"model_id"`
	Revision  string `json:"revision"`
	Dimension int    `json:"dimension"`
}
type FeatureExpression struct {
	Op      string              `json:"op"`
	Field   string              `json:"field,omitempty"`
	Value   json.RawMessage     `json:"value,omitempty"`
	Args    []FeatureExpression `json:"args,omitempty"`
	Mapping map[string]string   `json:"mapping,omitempty"`
	Default *string             `json:"default,omitempty"`
}
type FeatureError struct {
	Code    string `json:"code"`
	Row     *int   `json:"row,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (e *FeatureError) Error() string {
	where := ""
	if e.Row != nil {
		where = fmt.Sprintf(" at row %d", *e.Row)
	}
	return "feature " + e.Code + where + ": " + e.Message
}
func (e *FeatureError) Unwrap() error { return ErrInvalid }

type FeaturePreview struct {
	Values           []FeatureValue `json:"values"`
	TotalCount       int            `json:"total_count"`
	Warnings         []string       `json:"warnings"`
	DefinitionDigest string         `json:"definition_digest"`
	SourceDigest     string         `json:"source_digest"`
}
type FeatureMaterialization struct {
	ReceiptID        string    `json:"receipt_id"`
	FeatureID        string    `json:"feature_id"`
	Version          int       `json:"version"`
	SourceRefID      string    `json:"source_ref_id"`
	DefinitionDigest string    `json:"definition_digest"`
	SourceDigest     string    `json:"source_digest"`
	OutputRefID      string    `json:"output_ref_id"`
	Count            int       `json:"count"`
	AvailableAt      time.Time `json:"available_at"`
}

// FeatureMaterializationRepository atomically stores all values and the receipt.
// Implementations assign availability from their trusted database clock, ignore
// caller timestamps for availability, and preserve first-write history on replay.
type FeatureMaterializationRepository interface {
	FeatureMaterialize(context.Context, int64, int64, string, FeatureMaterialization, []FeatureValue) (*FeatureMaterialization, error)
	FeatureMaterializeGoverned(context.Context, int64, int64, string, FeatureMaterialization, []FeatureValue) (*FeatureMaterialization, error)
	FeatureReceipt(context.Context, int64, int64, string) (*FeatureMaterialization, error)
	FeatureReceiptByID(context.Context, int64, int64, string) (*FeatureMaterialization, error)
}
