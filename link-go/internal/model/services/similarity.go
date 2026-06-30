// Package services provides domain services that span multiple entities
package services

import (
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SimilarityMethod represents the method for calculating text similarity
type SimilarityMethod string

const (
	// SimilarityMethodJaccard uses Jaccard similarity coefficient
	SimilarityMethodJaccard SimilarityMethod = "jaccard"
	// SimilarityMethodCosine uses cosine similarity
	SimilarityMethodCosine SimilarityMethod = "cosine"
	// SimilarityMethodLevenshtein uses Levenshtein distance based similarity
	SimilarityMethodLevenshtein SimilarityMethod = "levenshtein"
)

// CalculateSimilarity calculates the similarity between two texts using the specified method
func CalculateSimilarity(text1, text2 string, method SimilarityMethod) float64 {
	switch method {
	case SimilarityMethodCosine:
		return cosineSimilarity(text1, text2)
	case SimilarityMethodLevenshtein:
		return levenshteinSimilarity(text1, text2)
	case SimilarityMethodJaccard:
		return jaccardSimilarity(text1, text2)
	default:
		return jaccardSimilarity(text1, text2)
	}
}

// jaccardSimilarity calculates Jaccard similarity coefficient
func jaccardSimilarity(text1, text2 string) float64 {
	set1 := wordSet(text1)
	set2 := wordSet(text2)

	if len(set1) == 0 && len(set2) == 0 {
		return 1.0
	}

	intersection := 0
	for word := range set1 {
		if set2[word] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

// cosineSimilarity calculates cosine similarity between two texts
func cosineSimilarity(text1, text2 string) float64 {
	vec1 := wordVector(text1)
	vec2 := wordVector(text2)

	if len(vec1) == 0 || len(vec2) == 0 {
		return 0.0
	}

	dotProduct := 0.0
	norm1 := 0.0
	norm2 := 0.0

	for word, count1 := range vec1 {
		count2 := vec2[word]
		dotProduct += float64(count1 * count2)
		norm1 += float64(count1 * count1)
	}

	for _, count := range vec2 {
		norm2 += float64(count * count)
	}

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

// levenshteinSimilarity calculates similarity based on Levenshtein distance
func levenshteinSimilarity(text1, text2 string) float64 {
	distance := levenshteinDistance(text1, text2)
	maxLen := max(len(text1), len(text2))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(distance)/float64(maxLen)
}

// wordSet converts text to a set of words
func wordSet(text string) map[string]bool {
	set := make(map[string]bool)
	words := tokenize(text)
	for _, word := range words {
		if word != "" {
			set[strings.ToLower(word)] = true
		}
	}
	return set
}

// wordVector converts text to a word vector (term frequency)
func wordVector(text string) map[string]int {
	vec := make(map[string]int)
	words := tokenize(text)
	for _, word := range words {
		if word != "" {
			vec[strings.ToLower(word)]++
		}
	}
	return vec
}

// tokenize splits text into words with Unicode normalization
func tokenize(text string) []string {
	// Unicode normalization
	t := transform.Chain(norm.NFD, runes.Remove(runes.Predicate(func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Remove accent marks
	})))
	normalized, _, _ := transform.String(t, text)

	// Simple tokenization: split by spaces and punctuation
	return strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	runes1 := []rune(s1)
	runes2 := []rune(s2)
	len1 := len(runes1)
	len2 := len(runes2)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	// Use two rows to save space
	prev := make([]int, len2+1)
	curr := make([]int, len2+1)

	for j := 0; j <= len2; j++ {
		prev[j] = j
	}

	for i := 1; i <= len1; i++ {
		curr[0] = i
		for j := 1; j <= len2; j++ {
			cost := 0
			if runes1[i-1] != runes2[j-1] {
				cost = 1
			}

			del := curr[j-1] + 1
			ins := prev[j] + 1
			sub := prev[j-1] + cost

			curr[j] = minInt(minInt(del, ins), sub)
		}
		prev, curr = curr, prev
	}

	return prev[len2]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
