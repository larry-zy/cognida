// Package collaboration: helpers for conflict detection and consensus voting.
package collaboration

import (
	"strings"
)

// voteSimilarityThreshold is the minimum token-Jaccard similarity for two
// answers to be considered as voting for the same consensus cluster.
const voteSimilarityThreshold = 0.35

// stopWords are common tokens excluded from topic comparison so that overlap
// reflects meaningful content rather than filler. Covers English + common
// Chinese function words.
var stopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "to": {}, "of": {}, "and": {}, "or": {}, "in": {}, "on": {},
	"at": {}, "for": {}, "with": {}, "as": {}, "by": {}, "it": {}, "this": {},
	"that": {}, "these": {}, "those": {}, "i": {}, "you": {}, "he": {}, "she": {},
	"we": {}, "they": {}, "but": {}, "if": {}, "then": {}, "so": {},
	"的": {}, "了": {}, "是": {}, "在": {}, "和": {}, "与": {}, "也": {}, "就": {},
	"都": {}, "而": {}, "及": {}, "或": {},
}

// succeededWithIDs returns succeeded sub-task results paired with their task ID.
func succeededWithIDs(result *ExecutionResult) []taggedResult {
	out := make([]taggedResult, 0, len(result.SubTasks))
	for id, r := range result.SubTasks {
		if r != nil && r.Error == "" {
			out = append(out, taggedResult{taskID: id, result: r})
		}
	}
	return out
}

// tokenSet splits content into a set of significant lowercase tokens.
func tokenSet(content string) map[string]struct{} {
	fields := strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		// Split on anything that is not a letter or digit.
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') &&
			!(r >= 'A' && r <= 'Z') && !(r >= 0x4e00 && r <= 0x9fff)
	})
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if len(f) == 0 {
			continue
		}
		if _, skip := stopWords[f]; skip {
			continue
		}
		set[f] = struct{}{}
	}
	return set
}

// jaccardSimilarity computes |A∩B| / |A∪B| for two token sets (0 when both empty).
func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for t := range a {
		if _, ok := b[t]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// containsAny reports whether s contains any of the provided substrings.
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// extractJSON strips markdown code fences and surrounding prose, returning the
// substring spanning the first JSON array/object to the matching last bracket.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` or ``` ... ``` fences.
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}

	start := strings.IndexAny(s, "[{")
	if start == -1 {
		return s
	}
	open := s[start]
	close := byte(']')
	if open == '{' {
		close = '}'
	}
	end := strings.LastIndexByte(s, close)
	if end <= start {
		return s
	}
	return s[start : end+1]
}

// ========================================
// Consensus Clustering
// ========================================

// answerCluster is a group of answers that agree with one another.
type answerCluster struct {
	members []taggedResult
}

func (c answerCluster) totalLen() int {
	total := 0
	for _, m := range c.members {
		total += len(m.result.Content)
	}
	return total
}

// representative returns the longest answer in the cluster.
func (c answerCluster) representative() taggedResult {
	rep := c.members[0]
	for _, m := range c.members[1:] {
		if len(m.result.Content) > len(rep.result.Content) {
			rep = m
		}
	}
	return rep
}

// clusterBySimilarity groups voters into clusters using greedy single-linkage
// on token-Jaccard similarity. A voter joins the first cluster whose seed it is
// similar enough to; otherwise it starts a new cluster.
func clusterBySimilarity(voters []taggedResult, threshold float64) []answerCluster {
	clusters := make([]answerCluster, 0)
	seeds := make([]map[string]struct{}, 0)

	for _, v := range voters {
		vSet := tokenSet(v.result.Content)
		placed := false
		for i := range clusters {
			if jaccardSimilarity(vSet, seeds[i]) >= threshold {
				clusters[i].members = append(clusters[i].members, v)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, answerCluster{members: []taggedResult{v}})
			seeds = append(seeds, vSet)
		}
	}
	return clusters
}
