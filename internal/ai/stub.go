package ai

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"
)

const Provider = "local-stub"

func Summarize(p domain.Patent, classifications []domain.Classification, citations []domain.CitationEdge) domain.AIArtifact {
	body := fmt.Sprintf("Local summary for %s: %s. Assignee: %s. Classification coverage: %s. Citation count in this database: %d. Abstract focus: %s",
		p.Number, p.Title, valueOrUnknown(p.Assignee), joinClassifications(classifications), len(citations), firstSentence(p.Abstract))
	return domain.AIArtifact{PatentNumber: p.Number, ArtifactType: "summary", Provider: Provider, Body: body}
}

func Compare(base, other domain.Patent) domain.AIArtifact {
	score := 0
	if base.Assignee != "" && strings.EqualFold(base.Assignee, other.Assignee) {
		score += 30
	}
	score += sharedWordScore(base.Title+" "+base.Abstract, other.Title+" "+other.Abstract)
	if score > 100 {
		score = 100
	}
	body := fmt.Sprintf("Local comparison: %s vs %s. Deterministic relevance score: %d/100. Shared terms are derived from title and abstract overlap; matching assignee adds weight.", base.Number, other.Number, score)
	return domain.AIArtifact{PatentNumber: base.Number, ArtifactType: "comparison", ComparedPatentNumber: other.Number, Provider: Provider, Body: body}
}

func joinClassifications(classifications []domain.Classification) string {
	if len(classifications) == 0 {
		return "unknown"
	}
	var codes []string
	for _, cls := range classifications {
		codes = append(codes, cls.Code)
	}
	return strings.Join(codes, ", ")
}

func firstSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "not available"
	}
	if idx := strings.Index(value, "."); idx > 0 {
		return value[:idx+1]
	}
	return value
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func sharedWordScore(a, b string) int {
	words := map[string]bool{}
	for _, word := range strings.Fields(strings.ToLower(a)) {
		if len(word) > 4 {
			words[strings.Trim(word, ".,;:()")] = true
		}
	}
	score := 0
	for _, word := range strings.Fields(strings.ToLower(b)) {
		key := strings.Trim(word, ".,;:()")
		if words[key] {
			score += 8
			delete(words, key)
		}
	}
	return score
}
