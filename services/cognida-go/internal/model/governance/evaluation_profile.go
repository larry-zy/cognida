package governance

// EvaluationProfile is part of the annotation that humans approve. Changing it
// uses UpdateAnnotation and consequently clears approval like any other label.
type EvaluationProfile struct {
	Kind             string   `json:"kind"`
	SourceRefs       []string `json:"source_refs,omitempty"`
	RelevantChunkIDs []string `json:"relevant_chunk_ids,omitempty"`
	ExpectedTools    []string `json:"expected_tools,omitempty"`
	ExpectedSteps    []string `json:"expected_steps,omitempty"`
	DatasourceID     string   `json:"datasource_id,omitempty"`
	Dialect          string   `json:"dialect,omitempty"`
	GoldSQL          string   `json:"gold_sql,omitempty"`
}
