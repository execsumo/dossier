package core

import (
	"strings"
	"time"
)

// Suggestion represents a candidate dossier suggestion with a confidence score.
type Suggestion struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Confidence string  `json:"confidence"` // "high", "medium", "low"
	Reason     string  `json:"reason"`
	Score      float64 `json:"score"` // internal numeric score for sorting
}

// ScoreDossier calculates a lexical similarity score between a query (like session content/name)
// and an existing Dossier.
func ScoreDossier(query string, d *Dossier, now time.Time) Suggestion {
	if query == "" {
		return Suggestion{ID: d.Frontmatter.ID, Name: d.Frontmatter.Name, Confidence: "low", Score: 0}
	}

	queryLower := strings.ToLower(query)
	queryTokens := tokenize(queryLower)
	if len(queryTokens) == 0 {
		return Suggestion{ID: d.Frontmatter.ID, Name: d.Frontmatter.Name, Confidence: "low", Score: 0}
	}

	nameLower := strings.ToLower(d.Frontmatter.Name)
	slugLower := strings.ToLower(d.Frontmatter.Slug)
	nextActionLower := strings.ToLower(d.Frontmatter.NextAction)
	bodyLower := strings.ToLower(d.DistilledState.Body)

	var score float64

	// 1. Weight exact name/slug match highest.
	for _, tok := range queryTokens {
		if tok == nameLower || tok == slugLower {
			score += 10.0
		} else if strings.Contains(nameLower, tok) {
			score += 5.0
		} else if strings.Contains(slugLower, tok) {
			score += 4.0
		}

		// 2. Weight next_action above the full Markdown body.
		if strings.Contains(nextActionLower, tok) {
			score += 3.0
		}

		// 3. Weight body text, including the Markdown Open Questions section.
		if strings.Contains(bodyLower, tok) {
			score += 1.0
		}
	}

	// Normalize score by the number of tokens in the query
	score /= float64(len(queryTokens))

	// 4. Weight recently updated Dossiers slightly above older ones.
	daysSinceUpdated := now.Sub(d.Frontmatter.UpdatedAt).Hours() / 24
	if daysSinceUpdated < 0 {
		daysSinceUpdated = 0
	}
	// Add a small recency bonus (max 1.0, decaying with age).
	recencyBonus := 1.0 / (1.0 + 0.1*daysSinceUpdated)
	score += recencyBonus

	// Determine confidence tier
	confidence := "low"
	reason := "Weak overlap."
	if score >= 5.0 {
		confidence = "high"
		reason = "Strong exact or repeated domain match."
	} else if score >= 1.5 {
		confidence = "medium"
		reason = "Plausible overlap."
	}

	return Suggestion{
		ID:         d.Frontmatter.ID,
		Slug:       d.Frontmatter.Slug,
		Name:       d.Frontmatter.Name,
		Status:     string(d.Frontmatter.Status),
		Confidence: confidence,
		Reason:     reason,
		Score:      score,
	}
}

func tokenize(s string) []string {
	// Simple word tokenizer removing stop words
	stopWords := map[string]bool{
		"a": true, "about": true, "above": true, "after": true, "again": true, "against": true,
		"all": true, "am": true, "an": true, "and": true, "any": true, "are": true, "aren't": true,
		"as": true, "at": true, "be": true, "because": true, "been": true, "before": true,
		"being": true, "below": true, "between": true, "both": true, "but": true, "by": true,
		"can't": true, "cannot": true, "could": true, "couldn't": true, "did": true, "didn't": true,
		"do": true, "does": true, "doesn't": true, "doing": true, "don't": true, "down": true,
		"during": true, "each": true, "few": true, "for": true, "from": true, "further": true,
		"had": true, "hadn't": true, "has": true, "hasn't": true, "have": true, "haven't": true,
		"having": true, "he": true, "he'd": true, "he'll": true, "he's": true, "her": true,
		"here": true, "here's": true, "hers": true, "herself": true, "him": true, "himself": true,
		"his": true, "how": true, "how's": true, "i": true, "i'd": true, "i'll": true, "i'm": true,
		"i've": true, "if": true, "in": true, "into": true, "is": true, "isn't": true, "it": true,
		"it's": true, "its": true, "itself": true, "let's": true, "me": true, "more": true,
		"most": true, "mustn't": true, "my": true, "myself": true, "no": true, "nor": true,
		"not": true, "of": true, "off": true, "on": true, "once": true, "only": true, "or": true,
		"other": true, "ought": true, "our": true, "ours": true, "ourselves": true, "out": true,
		"over": true, "own": true, "same": true, "shan't": true, "she": true, "she'd": true,
		"she'll": true, "she's": true, "should": true, "shouldn't": true, "so": true, "some": true,
		"such": true, "than": true, "that": true, "that's": true, "the": true, "their": true,
		"theirs": true, "them": true, "themselves": true, "then": true, "there": true, "there's": true,
		"these": true, "they": true, "they'd": true, "they'll": true, "they're": true, "they've": true,
		"this": true, "those": true, "through": true, "to": true, "too": true, "under": true,
		"until": true, "up": true, "very": true, "was": true, "wasn't": true, "we": true,
		"we'd": true, "we'll": true, "we're": true, "we've": true, "were": true, "weren't": true,
		"what": true, "what's": true, "when": true, "when's": true, "where": true, "where's": true,
		"which": true, "while": true, "who": true, "who's": true, "whom": true, "why": true,
		"why's": true, "with": true, "won't": true, "would": true, "wouldn't": true, "you": true,
		"you'd": true, "you'll": true, "you're": true, "you've": true, "your": true, "yours": true,
		"yourself": true, "yourselves": true,
	}

	var words []string
	var currentWord strings.Builder

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '\'' || r == '-' {
			currentWord.WriteRune(r)
		} else {
			if currentWord.Len() > 0 {
				w := currentWord.String()
				if !stopWords[w] {
					words = append(words, w)
				}
				currentWord.Reset()
			}
		}
	}
	if currentWord.Len() > 0 {
		w := currentWord.String()
		if !stopWords[w] {
			words = append(words, w)
		}
	}

	return words
}
