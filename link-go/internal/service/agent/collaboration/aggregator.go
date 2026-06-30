// Package collaboration provides result aggregation and synthesis for Multi-Agent collaboration.
package collaboration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	infraagent "link/internal/service/agent/framework"
)

// ========================================
// Result Aggregator (结果聚合器)
// ========================================

// ConflictType represents the type of conflict detected.
type ConflictType string

const (
	ConflictTypeContradiction ConflictType = "contradiction" // Direct contradiction
	ConflictTypeInconsistency ConflictType = "inconsistency" // Inconsistent information
	ConflictTypeAmbiguity     ConflictType = "ambiguity"     // Ambiguous or unclear
)

// Conflict represents a detected conflict between agent results.
type Conflict struct {
	Type        ConflictType           `json:"type"`
	Description string                 `json:"description"`
	Source1     string                 `json:"source1"` // Task ID or Agent ID
	Source2     string                 `json:"source2"`
	Content1    string                 `json:"content1"`
	Content2    string                 `json:"content2"`
	Severity    float64                `json:"severity"` // 0-1
	Resolution  string                 `json:"resolution,omitempty"`
}

// AggregationStrategy defines how to aggregate results.
type AggregationStrategy string

const (
	StrategyMergeAll     AggregationStrategy = "merge_all"     // Simply merge all results
	StrategySelectBest   AggregationStrategy = "select_best"   // Select the best result
	StrategySynthesize   AggregationStrategy = "synthesize"    // Use LLM to synthesize
	StrategyVote         AggregationStrategy = "vote"          // Vote on consensus
)

// ResultAggregator aggregates results from multiple agents.
type ResultAggregator struct {
	llm       model.ChatModel
	strategy  AggregationStrategy
}

// NewResultAggregator creates a new result aggregator.
func NewResultAggregator(llm model.ChatModel, strategy AggregationStrategy) *ResultAggregator {
	if strategy == "" {
		strategy = StrategySynthesize
	}
	return &ResultAggregator{
		llm:      llm,
		strategy: strategy,
	}
}

// Aggregate aggregates multiple task results into a final response.
func (a *ResultAggregator) Aggregate(ctx context.Context, result *ExecutionResult, originalQuery string) (*AggregatedResponse, error) {
	// Detect conflicts first
	conflicts := a.detectConflicts(ctx, result)

	response := &AggregatedResponse{
		OriginalQuery: originalQuery,
		PlanID:        result.PlanID,
		Conflicts:     conflicts,
		Strategy:      a.strategy,
	}

	switch a.strategy {
	case StrategyMergeAll:
		return a.mergeAll(ctx, result, response)
	case StrategySelectBest:
		return a.selectBest(ctx, result, response)
	case StrategySynthesize:
		return a.synthesize(ctx, result, response, originalQuery)
	case StrategyVote:
		return a.vote(ctx, result, response)
	default:
		return a.synthesize(ctx, result, response, originalQuery)
	}
}

// taggedResult pairs a sub-task ID with its result so conflict detection can
// reference the originating task.
type taggedResult struct {
	taskID string
	result *SubTaskResult
}

// detectConflicts detects conflicts between task results.
//
// It prefers an LLM-based semantic comparison when an LLM is configured, and
// transparently falls back to a deterministic heuristic when the LLM is
// unavailable or fails. This keeps the aggregator usable (and unit-testable)
// without a live model while still benefiting from semantic detection in
// production.
func (a *ResultAggregator) detectConflicts(ctx context.Context, result *ExecutionResult) []*Conflict {
	results := succeededWithIDs(result)
	if len(results) < 2 {
		return make([]*Conflict, 0)
	}

	if a.llm != nil {
		if conflicts, err := a.detectConflictsLLM(ctx, results); err == nil {
			return conflicts
		}
		// fall through to heuristic on error
	}

	return a.detectConflictsHeuristic(results)
}

// detectConflictsHeuristic performs pairwise heuristic conflict detection.
func (a *ResultAggregator) detectConflictsHeuristic(results []taggedResult) []*Conflict {
	conflicts := make([]*Conflict, 0)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if conflict := a.compareResults(results[i], results[j]); conflict != nil {
				conflicts = append(conflicts, conflict)
			}
		}
	}
	return conflicts
}

// compareResults compares two results for conflicts using polarity + topic overlap.
func (a *ResultAggregator) compareResults(r1, r2 taggedResult) *Conflict {
	content1 := strings.ToLower(r1.result.Content)
	content2 := strings.ToLower(r2.result.Content)

	if !a.hasContradiction(content1, content2) {
		return nil
	}

	// Severity scales with how much the two answers talk about the same topic:
	// a high-overlap contradiction is more likely a genuine conflict.
	severity := 0.5 + 0.5*jaccardSimilarity(tokenSet(content1), tokenSet(content2))
	if severity > 1 {
		severity = 1
	}

	return &Conflict{
		Type:        ConflictTypeContradiction,
		Description: "Detected opposing polarity on overlapping topic",
		Source1:     r1.taskID,
		Source2:     r2.taskID,
		Content1:    r1.result.Content,
		Content2:    r2.result.Content,
		Severity:    severity,
	}
}

// negationMarkers are tokens/phrases that flip the polarity of a statement.
var negationMarkers = []string{
	"not ", "no ", "never", "cannot", "can't", "won't", "isn't", "aren't",
	"false", "incorrect", "wrong", "unable", "impossible",
	"不", "没有", "无法", "不能", "不是", "错误", "否", "不可", "不会", "不可行",
}

// hasContradiction reports whether two contents contradict each other.
//
// Heuristic: the two contents must (a) discuss the same topic, approximated by
// significant-token overlap, and (b) carry opposite polarity, approximated by
// the presence of negation markers in one but not the other. This is
// intentionally conservative — it only fires when both signals agree.
func (a *ResultAggregator) hasContradiction(c1, c2 string) bool {
	overlap := jaccardSimilarity(tokenSet(c1), tokenSet(c2))
	if overlap < 0.2 {
		// Different topics — not a contradiction.
		return false
	}

	neg1 := containsAny(c1, negationMarkers)
	neg2 := containsAny(c2, negationMarkers)

	// Opposite polarity on a shared topic indicates a likely contradiction.
	return neg1 != neg2
}

// detectConflictsLLM asks the LLM to compare results and return structured conflicts.
func (a *ResultAggregator) detectConflictsLLM(ctx context.Context, results []taggedResult) ([]*Conflict, error) {
	var builder strings.Builder
	builder.WriteString("Compare the following agent results and identify any conflicts ")
	builder.WriteString("(contradiction, inconsistency, or ambiguity) between them.\n\n")
	for _, tr := range results {
		builder.WriteString(fmt.Sprintf("### Task %s\n%s\n\n", tr.taskID, tr.result.Content))
	}
	builder.WriteString(`Respond with ONLY a JSON array (no prose, no code fences). Each element:
{"source1":"<task id>","source2":"<task id>","type":"contradiction|inconsistency|ambiguity","description":"...","severity":0.0-1.0}
If there are no conflicts, respond with [].`)

	messages := []*schema.Message{
		schema.SystemMessage("You are a meticulous fact-checker that detects conflicts between sources. Output strict JSON only."),
		schema.UserMessage(builder.String()),
	}

	resp, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}

	var raw []struct {
		Source1     string  `json:"source1"`
		Source2     string  `json:"source2"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
		Severity    float64 `json:"severity"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &raw); err != nil {
		return nil, fmt.Errorf("parse conflict detection response: %w", err)
	}

	contentByID := make(map[string]string, len(results))
	for _, tr := range results {
		contentByID[tr.taskID] = tr.result.Content
	}

	conflicts := make([]*Conflict, 0, len(raw))
	for _, c := range raw {
		ct := ConflictType(c.Type)
		switch ct {
		case ConflictTypeContradiction, ConflictTypeInconsistency, ConflictTypeAmbiguity:
		default:
			ct = ConflictTypeInconsistency
		}
		conflicts = append(conflicts, &Conflict{
			Type:        ct,
			Description: c.Description,
			Source1:     c.Source1,
			Source2:     c.Source2,
			Content1:    contentByID[c.Source1],
			Content2:    contentByID[c.Source2],
			Severity:    c.Severity,
		})
	}

	return conflicts, nil
}

// mergeAll simply merges all results.
func (a *ResultAggregator) mergeAll(ctx context.Context, result *ExecutionResult, response *AggregatedResponse) (*AggregatedResponse, error) {
	var builder strings.Builder

	builder.WriteString("# 综合结果\n\n")
	builder.WriteString(fmt.Sprintf("共 %d 个子任务，%d 个成功，%d 个失败\n\n",
		result.TotalTasks,
		result.TotalTasks-result.FailedTasks,
		result.FailedTasks))

	for taskID, taskResult := range result.SubTasks {
		builder.WriteString(fmt.Sprintf("## %s\n", taskID))
		if taskResult.Error != "" {
			builder.WriteString(fmt.Sprintf("**失败**: %s\n\n", taskResult.Error))
		} else {
			builder.WriteString(fmt.Sprintf("%s\n\n", taskResult.Content))
		}
	}

	response.Content = builder.String()
	return response, nil
}

// selectBest selects the best result.
func (a *ResultAggregator) selectBest(ctx context.Context, result *ExecutionResult, response *AggregatedResponse) (*AggregatedResponse, error) {
	// Select the result with the most content (simple heuristic)
	var best *SubTaskResult
	var bestTaskID string

	for taskID, taskResult := range result.SubTasks {
		if taskResult.Error != "" {
			continue
		}
		if best == nil || len(taskResult.Content) > len(best.Content) {
			best = taskResult
			bestTaskID = taskID
		}
	}

	if best == nil {
		response.Content = "所有任务都失败了，无法生成结果。"
		return response, nil
	}

	response.Content = best.Content
	response.SelectedSource = bestTaskID
	return response, nil
}

// synthesize uses LLM to synthesize a comprehensive response.
func (a *ResultAggregator) synthesize(ctx context.Context, result *ExecutionResult, response *AggregatedResponse, originalQuery string) (*AggregatedResponse, error) {
	// Build synthesis prompt
	prompt := a.buildSynthesisPrompt(originalQuery, result)

	messages := []*schema.Message{
		schema.SystemMessage(`You are an expert at synthesizing information from multiple sources.

Your task is to combine results from different agents into a coherent, comprehensive response that directly answers the user's question.

Guidelines:
- Integrate information from all successful tasks
- Resolve any conflicts or inconsistencies
- Present a clear, well-structured answer
- If information is missing or conflicting, acknowledge it
- Use markdown formatting for better readability`),
		schema.UserMessage(prompt),
	}

	resp, err := a.llm.Generate(ctx, messages)
	if err != nil {
		// Fallback to merge all
		return a.mergeAll(ctx, result, response)
	}

	response.Content = resp.Content
	return response, nil
}

// buildSynthesisPrompt builds the prompt for synthesis.
func (a *ResultAggregator) buildSynthesisPrompt(originalQuery string, result *ExecutionResult) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("## Original Query\n%s\n\n", originalQuery))
	builder.WriteString("## Results from Sub-Tasks\n\n")

	for taskID, taskResult := range result.SubTasks {
		builder.WriteString(fmt.Sprintf("### Task: %s\n", taskID))
		if taskResult.Error != "" {
			builder.WriteString(fmt.Sprintf("**Failed**: %s\n", taskResult.Error))
		} else {
			builder.WriteString(taskResult.Content)
		}
		builder.WriteString("\n\n")
	}

	builder.WriteString("\nPlease synthesize these results into a comprehensive answer to the original query.")

	return builder.String()
}

// vote reaches consensus by clustering similar answers and electing the
// majority cluster.
//
// Each succeeded sub-task is one voter. Answers are grouped into clusters by
// topical similarity (token Jaccard above voteSimilarityThreshold); the largest
// cluster wins, ties broken by total content length. The winning cluster's
// representative (its longest answer) is returned, and when an LLM is available
// the winning answers are synthesized into a single coherent response. Vote
// distribution is recorded in response.Metadata for transparency.
func (a *ResultAggregator) vote(ctx context.Context, result *ExecutionResult, response *AggregatedResponse) (*AggregatedResponse, error) {
	voters := succeededWithIDs(result)
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}

	if len(voters) == 0 {
		response.Content = "所有任务都失败了，无法生成结果。"
		return response, nil
	}
	if len(voters) == 1 {
		response.Content = voters[0].result.Content
		response.SelectedSource = voters[0].taskID
		response.Metadata["total_votes"] = 1
		response.Metadata["winning_votes"] = 1
		return response, nil
	}

	clusters := clusterBySimilarity(voters, voteSimilarityThreshold)

	// Elect the winning cluster: most votes, ties broken by total content length.
	winner := clusters[0]
	for _, c := range clusters[1:] {
		if len(c.members) > len(winner.members) ||
			(len(c.members) == len(winner.members) && c.totalLen() > winner.totalLen()) {
			winner = c
		}
	}

	// Record vote distribution (sorted by votes desc for stable output).
	distribution := make([]map[string]interface{}, 0, len(clusters))
	for _, c := range clusters {
		ids := make([]string, 0, len(c.members))
		for _, m := range c.members {
			ids = append(ids, m.taskID)
		}
		distribution = append(distribution, map[string]interface{}{
			"votes": len(c.members),
			"tasks": ids,
		})
	}
	sort.SliceStable(distribution, func(i, j int) bool {
		return distribution[i]["votes"].(int) > distribution[j]["votes"].(int)
	})
	response.Metadata["vote_distribution"] = distribution
	response.Metadata["total_votes"] = len(voters)
	response.Metadata["winning_votes"] = len(winner.members)
	response.Metadata["cluster_count"] = len(clusters)

	rep := winner.representative()
	response.SelectedSource = rep.taskID

	// With a single dominant voter or no LLM, return the representative directly.
	if a.llm == nil || len(winner.members) == 1 {
		response.Content = rep.result.Content
		return response, nil
	}

	// Synthesize the winning cluster into one coherent answer.
	var builder strings.Builder
	builder.WriteString("Multiple agents reached a consensus answer. ")
	builder.WriteString("Synthesize the following agreeing responses into one clear, comprehensive answer.\n\n")
	for _, m := range winner.members {
		builder.WriteString(fmt.Sprintf("### Task %s\n%s\n\n", m.taskID, m.result.Content))
	}

	messages := []*schema.Message{
		schema.SystemMessage("You merge agreeing answers into a single authoritative response. Preserve all unique details."),
		schema.UserMessage(builder.String()),
	}
	resp, err := a.llm.Generate(ctx, messages)
	if err != nil {
		// Fallback to the representative answer.
		response.Content = rep.result.Content
		return response, nil
	}
	response.Content = resp.Content
	return response, nil
}

// ========================================
// Aggregated Response (聚合响应)
// ========================================

// AggregatedResponse represents the final aggregated response.
type AggregatedResponse struct {
	Content       string                 `json:"content"`
	OriginalQuery string                 `json:"original_query"`
	PlanID        string                 `json:"plan_id"`
	Conflicts     []*Conflict            `json:"conflicts,omitempty"`
	Strategy      AggregationStrategy    `json:"strategy"`
	SelectedSource string                `json:"selected_source,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ========================================
// Synthesizer Agent (综合 Agent)
// ========================================

// SynthesizerAgent is an agent that synthesizes results from multiple agents.
type SynthesizerAgent struct {
	llm     model.ChatModel
	aggregator *ResultAggregator
	name    string
}

// NewSynthesizerAgent creates a new synthesizer infraagent.
func NewSynthesizerAgent(llm model.ChatModel, strategy AggregationStrategy) *SynthesizerAgent {
	return &SynthesizerAgent{
		llm:        llm,
		aggregator: NewResultAggregator(llm, strategy),
		name:       "SynthesizerAgent",
	}
}

// Chat implements infraagent.Agent.
func (s *SynthesizerAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	// The synthesizer expects the message to contain an ExecutionResult
	// In practice, this would be called by the orchestrator

	// For direct chat, just respond normally
	return &infraagent.Response{
		Content: "SynthesizerAgent should be used through the CollaborativeAgent orchestrator.",
	}, nil
}

// Stream implements infraagent.Agent.
func (s *SynthesizerAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	ch := make(chan *infraagent.Chunk, 1)
	defer close(ch)

	return ch, nil
}

// Name implements infraagent.Agent.
func (s *SynthesizerAgent) Name() string {
	return s.name
}

// Synthesize is the main method for synthesizing results.
func (s *SynthesizerAgent) Synthesize(ctx context.Context, result *ExecutionResult, originalQuery string) (*AggregatedResponse, error) {
	return s.aggregator.Aggregate(ctx, result, originalQuery)
}

// ========================================
// Resolution Functions (冲突解决)
// ========================================

// ResolveConflicts attempts to resolve detected conflicts.
func (a *ResultAggregator) ResolveConflicts(ctx context.Context, response *AggregatedResponse) error {
	if len(response.Conflicts) == 0 {
		return nil
	}

	// Use LLM to resolve conflicts
	prompt := a.buildConflictResolutionPrompt(response)

	messages := []*schema.Message{
		schema.SystemMessage(`You are a mediator specializing in resolving conflicting information.

Your task is to analyze the conflicts and suggest resolutions or determine which information is more reliable.`),
		schema.UserMessage(prompt),
	}

	resp, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return err
	}

	// Update conflict resolutions
	for i, conflict := range response.Conflicts {
		if i == 0 {
			conflict.Resolution = resp.Content
		}
	}

	return nil
}

// buildConflictResolutionPrompt builds the prompt for conflict resolution.
func (a *ResultAggregator) buildConflictResolutionPrompt(response *AggregatedResponse) string {
	var builder strings.Builder

	builder.WriteString("## Conflicts Detected\n\n")

	for i, conflict := range response.Conflicts {
		builder.WriteString(fmt.Sprintf("### Conflict %d\n", i+1))
		builder.WriteString(fmt.Sprintf("**Type**: %s\n", conflict.Type))
		builder.WriteString(fmt.Sprintf("**Description**: %s\n", conflict.Description))
		builder.WriteString(fmt.Sprintf("**Source 1**: %s\n", conflict.Content1))
		builder.WriteString(fmt.Sprintf("**Source 2**: %s\n\n", conflict.Content2))
	}

	builder.WriteString("\nPlease analyze these conflicts and suggest how to resolve them.")
	builder.WriteString(" For each conflict, indicate which source is more reliable or how to reconcile the information.")

	return builder.String()
}
