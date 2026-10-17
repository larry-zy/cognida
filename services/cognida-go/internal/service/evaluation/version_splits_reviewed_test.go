package evaluation

import (
	d "cognida/internal/model/evaluation"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDatasetSplitsUseReviewedConsumerInput(t *testing.T) {
	boundary := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := boundary.Add(time.Hour)
	for _, profile := range []string{"qa", "rag", "agent", "sql"} {
		t.Run(profile, func(t *testing.T) {
			v := d.DatasetVersion{Profile: profile, Split: d.DatasetSplit{Algorithm: "source_time_v1", TrainBefore: &boundary, ValidationBefore: &end}, Samples: []d.ReviewedSample{
				{Content: "Original invoice material with distinct text", SourceTime: boundary.Add(-time.Hour), Group: "source-a"},
				{Content: "Unrelated wildlife observations from another publication", SourceTime: end.Add(time.Hour), Group: "source-b"},
			}, QAPairs: []d.QAPair{{Question: "How do I reset my password?", ReferenceAnswer: "Use settings"}, {Question: "How do I reset my password!", ReferenceAnswer: "A DIFFERENT answer does not prevent input leakage"}}}
			require.ErrorIs(t, assignDatasetSplits(&v), d.ErrVersionInvalid)
			v.QAPairs[1].Question = "What wildlife inhabits the southern wetlands?"
			require.NoError(t, assignDatasetSplits(&v))
			// Original evidence remains protected even after distinct human edits.
			v.Samples[1].Content = v.Samples[0].Content
			require.ErrorIs(t, assignDatasetSplits(&v), d.ErrVersionInvalid)
			v.Samples[1].SourceTime = v.Samples[0].SourceTime
			require.NoError(t, assignDatasetSplits(&v)) // duplicates within a split remain legal
		})
	}
}

func TestDatasetSplitsKeepActualTextInputForStructuredProfiles(t *testing.T) {
	boundary := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := boundary.Add(time.Hour)
	for _, profile := range []string{"classification", "named_entity", "relation"} {
		t.Run(profile, func(t *testing.T) {
			v := d.DatasetVersion{Profile: profile, Split: d.DatasetSplit{Algorithm: "source_time_v1", TrainBefore: &boundary, ValidationBefore: &end}, Samples: []d.ReviewedSample{
				{AnnotationType: profile, Content: "The exact text consumed by this profile", SourceTime: boundary.Add(-time.Hour), Group: "first"},
				{AnnotationType: profile, Content: "The exact text consumed by this profile!", SourceTime: end.Add(time.Hour), Group: "second"},
			}}
			require.ErrorIs(t, assignDatasetSplits(&v), d.ErrVersionInvalid)
			v.Samples[1].Content = "An unrelated observational record from another source"
			require.NoError(t, assignDatasetSplits(&v))
		})
	}
}
