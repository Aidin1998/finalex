// filepath: c:\Orbit CEX\pincex_unified\internal\compliance\aml\screening\fuzzy_matcher.go
package screening

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/agnivade/levenshtein"
	"go.uber.org/zap"
)

// FuzzyMatcher provides advanced fuzzy matching capabilities for sanctions screening
type FuzzyMatcher struct {
	logger *zap.SugaredLogger
	config FuzzyMatchConfig
}

// FuzzyMatchConfig defines configuration for fuzzy matching
type FuzzyMatchConfig struct {
	// Levenshtein distance thresholds
	ExactMatchThreshold    float64 `json:"exact_match_threshold"`    // 1.0
	FuzzyMatchThreshold    float64 `json:"fuzzy_match_threshold"`    // 0.85
	PhoneticMatchThreshold float64 `json:"phonetic_match_threshold"` // 0.80
	PartialMatchThreshold  float64 `json:"partial_match_threshold"`  // 0.75

	// Weighting factors for different match types
	NameWeight       float64 `json:"name_weight"`       // 0.6
	DOBWeight        float64 `json:"dob_weight"`        // 0.2
	AddressWeight    float64 `json:"address_weight"`    // 0.15
	IdentifierWeight float64 `json:"identifier_weight"` // 0.05

	// Advanced matching options
	EnableSoundex       bool `json:"enable_soundex"`
	EnableMetaphone     bool `json:"enable_metaphone"`
	EnableJaroWinkler   bool `json:"enable_jaro_winkler"`
	EnableNGramMatching bool `json:"enable_ngram_matching"`
	NGramSize           int  `json:"ngram_size"`

	// Character substitution patterns for common name variations
	CharacterSubstitutions map[string][]string `json:"character_substitutions"`

	// Common name prefixes/suffixes to normalize
	NamePrefixes []string `json:"name_prefixes"`
	NameSuffixes []string `json:"name_suffixes"`
}

// MatchResult represents the result of a fuzzy matching operation
type MatchResult struct {
	OverallScore    float64            `json:"overall_score"`
	NameScore       float64            `json:"name_score"`
	DOBScore        float64            `json:"dob_score"`
	AddressScore    float64            `json:"address_score"`
	IdentifierScore float64            `json:"identifier_score"`
	MatchType       string             `json:"match_type"`
	Details         map[string]float64 `json:"details"`
	Confidence      float64            `json:"confidence"`
}

// NewFuzzyMatcher creates a new fuzzy matcher with advanced algorithms
func NewFuzzyMatcher(logger *zap.SugaredLogger) *FuzzyMatcher {
	return &FuzzyMatcher{
		logger: logger,
		config: FuzzyMatchConfig{
			ExactMatchThreshold:    1.0,
			FuzzyMatchThreshold:    0.85,
			PhoneticMatchThreshold: 0.80,
			PartialMatchThreshold:  0.75,
			NameWeight:             0.6,
			DOBWeight:              0.2,
			AddressWeight:          0.15,
			IdentifierWeight:       0.05,
			EnableSoundex:          true,
			EnableMetaphone:        true,
			EnableJaroWinkler:      true,
			EnableNGramMatching:    true,
			NGramSize:              3,
			CharacterSubstitutions: map[string][]string{
				"ae": {"ä", "æ"},
				"oe": {"ö", "œ"},
				"ue": {"ü"},
				"ss": {"ß"},
				"c":  {"k", "ck"},
				"ph": {"f"},
				"y":  {"i"},
				"z":  {"s"},
			},
			NamePrefixes: []string{"mr", "mrs", "ms", "dr", "prof", "sir", "dame", "lord", "lady"},
			NameSuffixes: []string{"jr", "sr", "ii", "iii", "iv", "phd", "md", "esq"},
		},
	}
}

// MatchNames performs comprehensive name matching using multiple algorithms
func (fm *FuzzyMatcher) MatchNames(ctx context.Context, queryName string, targetName string, alternateNames []string) *MatchResult {
	result := &MatchResult{
		Details: make(map[string]float64),
	}

	// Normalize names
	normalizedQuery := fm.normalizeName(queryName)
	normalizedTarget := fm.normalizeName(targetName)

	// Test against primary name
	primaryScore := fm.calculateNameScore(normalizedQuery, normalizedTarget)
	result.Details["primary_name"] = primaryScore

	// Test against alternate names
	var bestAlternateScore float64
	for i, altName := range alternateNames {
		normalizedAlt := fm.normalizeName(altName)
		altScore := fm.calculateNameScore(normalizedQuery, normalizedAlt)
		result.Details[fmt.Sprintf("alternate_name_%d", i)] = altScore
		if altScore > bestAlternateScore {
			bestAlternateScore = altScore
		}
	}

	// Use the best score
	result.NameScore = math.Max(primaryScore, bestAlternateScore)

	// Determine match type
	if result.NameScore >= fm.config.ExactMatchThreshold {
		result.MatchType = "exact"
	} else if result.NameScore >= fm.config.FuzzyMatchThreshold {
		result.MatchType = "fuzzy"
	} else if result.NameScore >= fm.config.PhoneticMatchThreshold {
		result.MatchType = "phonetic"
	} else if result.NameScore >= fm.config.PartialMatchThreshold {
		result.MatchType = "partial"
	} else {
		result.MatchType = "no_match"
	}

	result.OverallScore = result.NameScore
	result.Confidence = fm.calculateConfidence(result)

	return result
}

// calculateNameScore computes a comprehensive name similarity score
func (fm *FuzzyMatcher) calculateNameScore(name1, name2 string) float64 {
	if name1 == name2 {
		return 1.0
	}

	scores := make([]float64, 0)

	// 1. Levenshtein distance
	levScore := fm.levenshteinSimilarity(name1, name2)
	scores = append(scores, levScore)

	// 2. Jaro-Winkler similarity
	if fm.config.EnableJaroWinkler {
		jwScore := fm.jaroWinklerSimilarity(name1, name2)
		scores = append(scores, jwScore)
	}

	// 3. Soundex matching
	if fm.config.EnableSoundex {
		soundexScore := fm.soundexSimilarity(name1, name2)
		scores = append(scores, soundexScore)
	}

	// 4. Metaphone matching
	if fm.config.EnableMetaphone {
		metaphoneScore := fm.metaphoneSimilarity(name1, name2)
		scores = append(scores, metaphoneScore)
	}

	// 5. N-gram matching
	if fm.config.EnableNGramMatching {
		ngramScore := fm.ngramSimilarity(name1, name2)
		scores = append(scores, ngramScore)
	}

	// 6. Token-based matching
	tokenScore := fm.tokenSimilarity(name1, name2)
	scores = append(scores, tokenScore)

	// Return weighted average of all scores
	return fm.weightedAverage(scores)
}

// normalizeName normalizes a name for matching
func (fm *FuzzyMatcher) normalizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Remove special characters and keep only alphanumeric and spaces
	reg := regexp.MustCompile(`[^a-z0-9\s]`)
	name = reg.ReplaceAllString(name, "")

	// Apply character substitutions
	for standard, alternatives := range fm.config.CharacterSubstitutions {
		for _, alt := range alternatives {
			name = strings.ReplaceAll(name, alt, standard)
		}
	}

	// Remove common prefixes and suffixes
	tokens := strings.Fields(name)
	filteredTokens := make([]string, 0, len(tokens))

	for _, token := range tokens {
		// Skip common prefixes/suffixes
		if !fm.isCommonAffix(token) {
			filteredTokens = append(filteredTokens, token)
		}
	}

	// Rejoin and normalize whitespace
	name = strings.Join(filteredTokens, " ")
	name = strings.TrimSpace(name)

	return name
}

// isCommonAffix checks if a token is a common prefix or suffix
func (fm *FuzzyMatcher) isCommonAffix(token string) bool {
	token = strings.ToLower(token)

	for _, prefix := range fm.config.NamePrefixes {
		if token == prefix {
			return true
		}
	}

	for _, suffix := range fm.config.NameSuffixes {
		if token == suffix {
			return true
		}
	}

	return false
}

// levenshteinSimilarity calculates similarity using Levenshtein distance
func (fm *FuzzyMatcher) levenshteinSimilarity(s1, s2 string) float64 {
	distance := levenshtein.ComputeDistance(s1, s2)
	maxLen := math.Max(float64(len(s1)), float64(len(s2)))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - (float64(distance) / maxLen)
}

// jaroWinklerSimilarity implements the Jaro-Winkler algorithm
func (fm *FuzzyMatcher) jaroWinklerSimilarity(s1, s2 string) float64 {
	// Simplified Jaro-Winkler implementation
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Calculate Jaro similarity
	matchWindow := int(math.Max(float64(len1), float64(len2))/2) - 1
	if matchWindow < 0 {
		matchWindow = 0
	}

	s1Matches := make([]bool, len1)
	s2Matches := make([]bool, len2)

	matches := 0
	transpositions := 0

	// Find matches
	for i := 0; i < len1; i++ {
		start := int(math.Max(0, float64(i-matchWindow)))
		end := int(math.Min(float64(len2), float64(i+matchWindow+1)))

		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	// Find transpositions
	k := 0
	for i := 0; i < len1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	jaro := (float64(matches)/float64(len1) + float64(matches)/float64(len2) +
		(float64(matches)-float64(transpositions)/2)/float64(matches)) / 3.0

	// Calculate Winkler prefix bonus
	prefix := 0
	for i := 0; i < int(math.Min(float64(len1), float64(len2))) && i < 4; i++ {
		if s1[i] == s2[i] {
			prefix++
		} else {
			break
		}
	}

	return jaro + (0.1 * float64(prefix) * (1.0 - jaro))
}

// soundexSimilarity computes similarity using Soundex algorithm
func (fm *FuzzyMatcher) soundexSimilarity(s1, s2 string) float64 {
	soundex1 := fm.soundex(s1)
	soundex2 := fm.soundex(s2)

	if soundex1 == soundex2 {
		return 1.0
	}

	return 0.0
}

// soundex implements the Soundex phonetic algorithm
func (fm *FuzzyMatcher) soundex(s string) string {
	if len(s) == 0 {
		return ""
	}

	s = strings.ToUpper(s)
	result := string(s[0])

	// Soundex mapping
	mapping := map[rune]rune{
		'B': '1', 'F': '1', 'P': '1', 'V': '1',
		'C': '2', 'G': '2', 'J': '2', 'K': '2', 'Q': '2', 'S': '2', 'X': '2', 'Z': '2',
		'D': '3', 'T': '3',
		'L': '4',
		'M': '5', 'N': '5',
		'R': '6',
	}

	var prev rune
	for _, char := range s[1:] {
		if code, exists := mapping[char]; exists {
			if code != prev {
				result += string(code)
				prev = code
			}
		} else {
			prev = 0
		}

		if len(result) >= 4 {
			break
		}
	}

	// Pad or truncate to 4 characters
	for len(result) < 4 {
		result += "0"
	}

	return result[:4]
}

// metaphoneSimilarity computes similarity using Double Metaphone algorithm (simplified)
func (fm *FuzzyMatcher) metaphoneSimilarity(s1, s2 string) float64 {
	metaphone1 := fm.metaphone(s1)
	metaphone2 := fm.metaphone(s2)

	if metaphone1 == metaphone2 {
		return 1.0
	}

	return 0.0
}

// metaphone implements a simplified Metaphone algorithm
func (fm *FuzzyMatcher) metaphone(s string) string {
	if len(s) == 0 {
		return ""
	}

	s = strings.ToUpper(s)
	result := ""

	// Simple metaphone rules (simplified implementation)
	for i, char := range s {
		switch char {
		case 'A', 'E', 'I', 'O', 'U':
			if i == 0 {
				result += string(char)
			}
		case 'B':
			if i == len(s)-1 && s[i-1] == 'M' {
				continue
			}
			result += "B"
		case 'C':
			if i > 0 && s[i-1] == 'S' && (i+1 < len(s)) && (s[i+1] == 'I' || s[i+1] == 'E') {
				continue
			}
			if (i+1 < len(s)) && s[i+1] == 'H' {
				result += "X"
			} else if (i+1 < len(s)) && (s[i+1] == 'I' || s[i+1] == 'E') {
				result += "S"
			} else {
				result += "K"
			}
		case 'D':
			if (i+2 < len(s)) && s[i+1] == 'G' && (s[i+2] == 'E' || s[i+2] == 'Y' || s[i+2] == 'I') {
				result += "J"
			} else {
				result += "T"
			}
		case 'F', 'J', 'L', 'M', 'N', 'R':
			result += string(char)
		case 'G':
			if (i+1 < len(s)) && s[i+1] == 'H' && i > 0 {
				continue
			}
			if (i+1 < len(s)) && s[i+1] == 'N' {
				continue
			}
			if (i+3 < len(s)) && s[i:i+4] == "GNED" {
				continue
			}
			result += "K"
		case 'H':
			if i == 0 || (i > 0 && !isVowel(rune(s[i-1]))) && (i+1 >= len(s) || !isVowel(rune(s[i+1]))) {
				result += "H"
			}
		case 'K':
			if i > 0 && s[i-1] != 'C' {
				result += "K"
			}
		case 'P':
			if (i+1 < len(s)) && s[i+1] == 'H' {
				result += "F"
			} else {
				result += "P"
			}
		case 'Q':
			result += "K"
		case 'S':
			if (i+2 < len(s)) && s[i+1] == 'H' {
				result += "X"
			} else if (i+2 < len(s)) && s[i+1] == 'I' && (s[i+2] == 'O' || s[i+2] == 'A') {
				result += "X"
			} else {
				result += "S"
			}
		case 'T':
			if (i+2 < len(s)) && s[i+1] == 'I' && (s[i+2] == 'O' || s[i+2] == 'A') {
				result += "X"
			} else if (i+1 < len(s)) && s[i+1] == 'H' {
				result += "0"
			} else if !((i+2 < len(s)) && s[i+1] == 'C' && s[i+2] == 'H') {
				result += "T"
			}
		case 'V':
			result += "F"
		case 'W', 'Y':
			if (i+1 < len(s)) && isVowel(rune(s[i+1])) {
				result += string(char)
			}
		case 'X':
			result += "KS"
		case 'Z':
			result += "S"
		}
	}

	return result
}

// isVowel checks if a character is a vowel
func isVowel(char rune) bool {
	vowels := "AEIOU"
	return strings.ContainsRune(vowels, char)
}

// ngramSimilarity computes similarity using N-gram analysis
func (fm *FuzzyMatcher) ngramSimilarity(s1, s2 string) float64 {
	ngrams1 := fm.generateNGrams(s1, fm.config.NGramSize)
	ngrams2 := fm.generateNGrams(s2, fm.config.NGramSize)

	if len(ngrams1) == 0 && len(ngrams2) == 0 {
		return 1.0
	}
	if len(ngrams1) == 0 || len(ngrams2) == 0 {
		return 0.0
	}

	intersection := 0
	for ngram := range ngrams1 {
		if ngrams2[ngram] {
			intersection++
		}
	}

	union := len(ngrams1) + len(ngrams2) - intersection
	return float64(intersection) / float64(union)
}

// generateNGrams generates n-grams for a string
func (fm *FuzzyMatcher) generateNGrams(s string, n int) map[string]bool {
	ngrams := make(map[string]bool)

	if len(s) < n {
		ngrams[s] = true
		return ngrams
	}

	for i := 0; i <= len(s)-n; i++ {
		ngram := s[i : i+n]
		ngrams[ngram] = true
	}

	return ngrams
}

// tokenSimilarity computes similarity using token-based analysis
func (fm *FuzzyMatcher) tokenSimilarity(s1, s2 string) float64 {
	tokens1 := strings.Fields(s1)
	tokens2 := strings.Fields(s2)

	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	// Create token sets
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)

	for _, token := range tokens1 {
		set1[token] = true
	}
	for _, token := range tokens2 {
		set2[token] = true
	}

	// Calculate Jaccard similarity
	intersection := 0
	for token := range set1 {
		if set2[token] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	return float64(intersection) / float64(union)
}

// weightedAverage calculates the weighted average of scores
func (fm *FuzzyMatcher) weightedAverage(scores []float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}

	// Sort scores in descending order and weight higher scores more
	sort.Slice(scores, func(i, j int) bool {
		return scores[i] > scores[j]
	})

	weightSum := 0.0
	weightedSum := 0.0

	for i, score := range scores {
		weight := 1.0 / (float64(i) + 1.0) // Diminishing weights
		weightedSum += score * weight
		weightSum += weight
	}

	return weightedSum / weightSum
}

// calculateConfidence estimates the confidence of a match result
func (fm *FuzzyMatcher) calculateConfidence(result *MatchResult) float64 {
	// Base confidence on score and match type
	baseConfidence := result.OverallScore

	// Adjust based on match type
	switch result.MatchType {
	case "exact":
		return math.Min(baseConfidence+0.1, 1.0)
	case "fuzzy":
		return baseConfidence
	case "phonetic":
		return baseConfidence * 0.9
	case "partial":
		return baseConfidence * 0.8
	default:
		return baseConfidence * 0.7
	}
}
