package lifecycle

import d "cognida/internal/model/lifecycle"

var agentOrder = []string{"data-access", "data-quality", "data-cleaning", "data-enrichment", "data-asset", "data-analysis", "data-evaluation"}

func Contracts() map[string]d.AgentContract {
	data := []d.Kind{d.RawAsset, d.CleanedAsset, d.EnrichedAsset, d.PublishedAsset}
	contracts := []d.AgentContract{
		{ID: "data-access", Name: "Data Access Agent", Purpose: "Validate acquired input and preserve provenance", InputKinds: data, OutputKinds: []d.Kind{d.RawAsset}, ToolAllowlist: []string{"asset.resolve", "access.acquire"}},
		{ID: "data-quality", Name: "Data Quality Agent", Purpose: "Profile data and produce evidence-backed quality gates", InputKinds: data, OutputKinds: append([]d.Kind{d.QualityReport}, data...), ToolAllowlist: []string{"asset.resolve", "quality.profile"}},
		{ID: "data-cleaning", Name: "Data Cleaning Agent", Purpose: "Create an immutable cleaned version and recheck quality", InputKinds: append([]d.Kind{d.QualityReport}, data...), OutputKinds: []d.Kind{d.CleanedAsset, d.QualityReport}, ToolAllowlist: []string{"asset.resolve", "cleaning.transform", "quality.profile"}},
		{ID: "data-enrichment", Name: "Data Enrichment Agent", Purpose: "Produce provenance-backed metadata recommendations", InputKinds: []d.Kind{d.RawAsset, d.CleanedAsset}, OutputKinds: []d.Kind{d.EnrichedAsset}, ToolAllowlist: []string{"asset.resolve", "enrichment.suggest"}},
		{ID: "data-asset", Name: "Data Asset Agent", Purpose: "Publish approved assets to knowledge or catalog", InputKinds: []d.Kind{d.RawAsset, d.CleanedAsset, d.EnrichedAsset}, OutputKinds: []d.Kind{d.PublishedAsset}, ToolAllowlist: []string{"asset.resolve", "asset.publish"}, RiskClass: "write"},
		{ID: "data-analysis", Name: "Data Analysis Agent", Purpose: "Read published assets and produce descriptive analysis", InputKinds: []d.Kind{d.PublishedAsset}, OutputKinds: []d.Kind{d.Insight}, ToolAllowlist: []string{"asset.resolve", "analysis.describe"}},
		{ID: "data-evaluation", Name: "Data Evaluation Agent", Purpose: "Evaluate evidence coverage and validity without inventing model accuracy", InputKinds: []d.Kind{d.Insight, d.PublishedAsset, d.QualityReport}, OutputKinds: []d.Kind{d.Evaluation}, ToolAllowlist: []string{"asset.resolve", "evaluation.inspect"}},
	}
	out := map[string]d.AgentContract{}
	for _, c := range contracts {
		c.SchemaVersion = "1"
		c.InvocationModes = []string{"standalone", "pipeline"}
		c.RequiredScopes = []string{"read"}
		if c.RiskClass == "" {
			c.RiskClass = "read"
		}
		c.MaxIterations = 1
		c.TokenBudget = 8000
		out[c.ID] = c
	}
	return out
}
func DefaultTemplate() d.PipelineDefinition {
	keys := []string{"access", "quality", "cleaning", "enrichment", "publication", "analysis", "evaluation"}
	stages := []d.StageDefinition{}
	for i, id := range agentOrder {
		st := d.StageDefinition{ID: keys[i], AgentID: id, MaxAttempts: 3, AllowedGates: []string{d.GatePassed}}
		if i > 0 {
			st.DependsOn = []string{keys[i-1]}
		}
		if id == "data-cleaning" {
			st.AllowedGates = append(st.AllowedGates, d.GateFailed)
		}
		if id == "data-asset" {
			st.Approval = true
		}
		stages = append(stages, st)
	}
	return d.PipelineDefinition{ID: "web-knowledge-to-insight", Version: 1, Name: "Web knowledge to insight", Stages: stages, MaxStages: 7, Enabled: true}
}
