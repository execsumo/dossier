package core

import (
	"sort"
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

type dossierScorer struct {
	queryTokens []string
	now         time.Time
}

func keepTopSuggestions(current []Suggestion, candidate Suggestion, limit int) []Suggestion {
	current = append(current, candidate)
	sort.Slice(current, func(i, j int) bool { return current[i].Score > current[j].Score })
	if len(current) > limit {
		current = current[:limit]
	}
	return current
}

func newDossierScorer(query string, now time.Time) dossierScorer {
	return dossierScorer{queryTokens: tokenize(strings.ToLower(query)), now: now}
}

// ScoreDossier calculates a lexical similarity score between a query (like session content/name)
// and an existing Dossier.
func ScoreDossier(query string, d *Dossier, now time.Time) Suggestion {
	return newDossierScorer(query, now).Score(d)
}

func (s dossierScorer) Score(d *Dossier) Suggestion {
	if len(s.queryTokens) == 0 {
		return Suggestion{ID: d.Frontmatter.ID, Name: d.Frontmatter.Name, Confidence: "low", Score: 0}
	}

	nameLower := strings.ToLower(d.Frontmatter.Name)
	slugLower := strings.ToLower(d.Frontmatter.Slug)
	nextActionLower := strings.ToLower(d.Frontmatter.NextAction)
	bodyLower := strings.ToLower(d.DistilledState.Body)

	var score float64

	// 1. Weight exact name/slug match highest.
	for _, tok := range s.queryTokens {
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
	score /= float64(len(s.queryTokens))

	// 4. Weight recently updated Dossiers slightly above older ones.
	daysSinceUpdated := s.now.Sub(d.Frontmatter.UpdatedAt).Hours() / 24
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

var stopWords = map[string]struct{}{
	"a": {}, "about": {}, "above": {}, "after": {}, "again": {}, "against": {},
	"all": {}, "am": {}, "an": {}, "and": {}, "any": {}, "are": {}, "aren't": {},
	"as": {}, "at": {}, "be": {}, "because": {}, "been": {}, "before": {},
	"being": {}, "below": {}, "between": {}, "both": {}, "but": {}, "by": {},
	"can't": {}, "cannot": {}, "could": {}, "couldn't": {}, "did": {}, "didn't": {},
	"do": {}, "does": {}, "doesn't": {}, "doing": {}, "don't": {}, "down": {},
	"during": {}, "each": {}, "few": {}, "for": {}, "from": {}, "further": {},
	"had": {}, "hadn't": {}, "has": {}, "hasn't": {}, "have": {}, "haven't": {},
	"having": {}, "he": {}, "he'd": {}, "he'll": {}, "he's": {}, "her": {},
	"here": {}, "here's": {}, "hers": {}, "herself": {}, "him": {}, "himself": {},
	"his": {}, "how": {}, "how's": {}, "i": {}, "i'd": {}, "i'll": {}, "i'm": {},
	"i've": {}, "if": {}, "in": {}, "into": {}, "is": {}, "isn't": {}, "it": {},
	"it's": {}, "its": {}, "itself": {}, "let's": {}, "me": {}, "more": {},
	"most": {}, "mustn't": {}, "my": {}, "myself": {}, "no": {}, "nor": {},
	"not": {}, "of": {}, "off": {}, "on": {}, "once": {}, "only": {}, "or": {},
	"other": {}, "ought": {}, "our": {}, "ours": {}, "ourselves": {}, "out": {},
	"over": {}, "own": {}, "same": {}, "shan't": {}, "she": {}, "she'd": {},
	"she'll": {}, "she's": {}, "should": {}, "shouldn't": {}, "so": {}, "some": {},
	"such": {}, "than": {}, "that": {}, "that's": {}, "the": {}, "their": {},
	"theirs": {}, "them": {}, "themselves": {}, "then": {}, "there": {}, "there's": {},
	"these": {}, "they": {}, "they'd": {}, "they'll": {}, "they're": {}, "they've": {},
	"this": {}, "those": {}, "through": {}, "to": {}, "too": {}, "under": {},
	"until": {}, "up": {}, "very": {}, "was": {}, "wasn't": {}, "we": {},
	"we'd": {}, "we'll": {}, "we're": {}, "we've": {}, "were": {}, "weren't": {},
	"what": {}, "what's": {}, "when": {}, "when's": {}, "where": {}, "where's": {},
	"which": {}, "while": {}, "who": {}, "who's": {}, "whom": {}, "why": {},
	"why's": {}, "with": {}, "won't": {}, "would": {}, "wouldn't": {}, "you": {},
	"you'd": {}, "you'll": {}, "you're": {}, "you've": {}, "your": {}, "yours": {},
	"yourself": {}, "yourselves": {},
}

func tokenize(s string) []string {

	var words []string
	var currentWord strings.Builder

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '\'' || r == '-' {
			currentWord.WriteRune(r)
		} else {
			if currentWord.Len() > 0 {
				w := currentWord.String()
				if _, stopped := stopWords[w]; !stopped {
					words = append(words, w)
				}
				currentWord.Reset()
			}
		}
	}
	if currentWord.Len() > 0 {
		w := currentWord.String()
		if _, stopped := stopWords[w]; !stopped {
			words = append(words, w)
		}
	}

	return words
}
