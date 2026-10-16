package lifecycle

import (
	"encoding/hex"
	"strings"
)

// Preparation is assigned only by the standardization executor after cleaning.
// It binds the immutable text that downstream annotations use for their offsets.
type Preparation struct {
	Version           int                        `json:"version"`
	SourceRefID       string                     `json:"source_ref_id"`
	SourceDigest      string                     `json:"source_digest"`
	OutputDigest      string                     `json:"output_digest"`
	SchemaDigest      string                     `json:"schema_digest"`
	Rows              int                        `json:"rows"`
	Fields            []PreparationField         `json:"fields"`
	NormalizeNewlines bool                       `json:"normalize_newlines"`
	TextProjection    *PreparationTextProjection `json:"text_projection,omitempty"`
}

type PreparationField struct {
	Source   string `json:"source"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type PreparationOptions struct {
	Fields            []PreparationField         `json:"fields,omitempty"`
	NormalizeNewlines bool                       `json:"normalize_newlines,omitempty"`
	TextProjection    *PreparationTextProjection `json:"text_projection,omitempty"`
}

// PreparationTextProjection selects source documents from a JSON array column.
// It is an explicit data mapping, not a query language or generated summary.
type PreparationTextProjection struct {
	Column    string `json:"column"`
	TextField string `json:"text_field"`
	IDColumn  string `json:"id_column,omitempty"`
}

// IsPreparedAsset checks a server-resolved asset, never caller-supplied metadata.
// Resolve must still validate the content digest and current parent authority.
func IsPreparedAsset(ref DataAssetRef) bool {
	p := ref.Provenance.Preparation
	if ref.Kind != CleanedAsset || p == nil || p.Version != 1 || p.Rows < 1 || p.Rows > 10000 || p.OutputDigest != ref.Provenance.Digest {
		return false
	}
	for _, digest := range []string{p.SourceDigest, p.OutputDigest, p.SchemaDigest} {
		if len(digest) != 64 || digest != strings.ToLower(digest) {
			return false
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return false
		}
	}
	source, err := ParseRefID(p.SourceRefID)
	return err == nil && len(ref.Parents) == 1 && ref.Parents[0] == source
}
