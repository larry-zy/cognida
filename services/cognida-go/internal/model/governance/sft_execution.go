package governance

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const SFTProfile = "governance-sft-candidate-v1"

// Implementations return the database instant inside the same transaction that
// owns the operation row; process clocks cannot authorize a late commit.
type GovernedCommitClock interface {
	GovernedNow(context.Context) (time.Time, error)
}

// SFTBuildParameters excludes source identity: the controller binds its actual
// authorized input separately. It cannot approve or publish a dataset.
type SFTBuildParameters struct {
	Name              string     `json:"name"`
	StandardVersionID string     `json:"standard_version_id"`
	Format            string     `json:"format"`
	Columns           SFTColumns `json:"columns"`
}

func (p SFTBuildParameters) Validate() error {
	if strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 128 || p.StandardVersionID == "" || len(p.StandardVersionID) > 64 {
		return ErrInvalid
	}
	c := p.Columns
	for _, col := range []string{c.Question, c.ReferenceAnswer, c.Messages, c.Tools, c.Split, c.Group} {
		if utf8.RuneCountInString(col) > 200 || col != strings.TrimSpace(col) {
			return ErrInvalid
		}
	}
	switch p.Format {
	case "qa":
		if c.Question == "" || c.ReferenceAnswer == "" || c.Question == c.ReferenceAnswer || c.Messages != "" || c.Tools != "" {
			return ErrInvalid
		}
	case "messages":
		if c.Messages == "" || c.Question != "" || c.ReferenceAnswer != "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (p SFTBuildParameters) Bind(source l.RefID) SFTInput {
	return SFTInput{Name: p.Name, SourceRefID: source.String(), StandardVersionID: p.StandardVersionID, Format: p.Format, Columns: p.Columns}
}
