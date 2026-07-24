package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hollo/inbox-guard/filters"
)

// activityObject is the minimal ActivityPub object we inspect.
type activityObject struct {
	Type  string `json:"type"`
	Actor string `json:"actor"`
	Object struct {
		Content string `json:"content"`
		Type    string `json:"type"`
	} `json:"object"`
}

// getContent unwraps Create/Announce activities to get the inner object content.
func getContent(body []byte) (string, string) {
	var act activityObject
	if err := json.Unmarshal(body, &act); err != nil {
		return "", ""
	}

	content := act.Object.Content
	if content == "" {
		// Top-level might be the object itself (e.g. Note)
		json.Unmarshal(body, &act.Object)
		content = act.Object.Content
	}

	return content, act.Actor
}

// countMentions counts <span class="h-card"> occurrences in HTML content.
func countMentions(content string) int {
	return strings.Count(content, `class="h-card"`)
}

// nonMentionLength estimates non-mention text length by stripping HTML tags
// and counting characters that are not part of mention markup.
func nonMentionContent(content string) int {
	// Strip all HTML tags first
	text := stripTags(content)
	// Remove mention handles (@user@domain)
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(text)
}

func stripTags(html string) string {
	inTag := false
	var b strings.Builder
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// filterChain holds all registered filters.
type filterChain []filters.Filter

// buildFilterChain creates the chain from config.
func buildFilterChain(cfg config) filterChain {
	var chain filterChain

	if cfg.maxMentions > 0 {
		chain = append(chain, &MentionFilter{maxMentions: cfg.maxMentions, maxRatio: cfg.maxContentRatio})
	}
	if len(cfg.blockKeywords) > 0 {
		chain = append(chain, &KeywordFilter{keywords: cfg.blockKeywords})
	}
	if len(cfg.blockDomains) > 0 {
		chain = append(chain, &DomainFilter{domains: cfg.blockDomains})
	}

	return chain
}

// Check runs all filters in sequence. Returns the first blocking reason.
func (fc filterChain) Check(body []byte, r *http.Request) (string, bool) {
	if len(fc) == 0 {
		return "", false
	}

	content, actor := getContent(body)

	for _, f := range fc {
		if reason := f.Check(content, actor, r); reason != "" {
			return reason, true
		}
	}
	return "", false
}

// ── MentionFilter ──────────────────────────────────────────────────────────

type MentionFilter struct {
	maxMentions int
	maxRatio    float64
}

func (f *MentionFilter) Check(content, actor string, r *http.Request) string {
	if content == "" {
		return ""
	}
	mentions := countMentions(content)
	if mentions == 0 {
		return ""
	}

	if mentions > f.maxMentions {
		return filters.Reason("mentions", "count", mentions, "max", f.maxMentions)
	}

	// Check ratio: if >90% of content is mentions, it's spam even with fewer mentions
	nonMention := nonMentionContent(content)
	total := mentions*50 + nonMention
	if total > 0 && float64(mentions*50)/float64(total) > f.maxRatio && mentions > 10 {
		return filters.Reason("content_ratio", "mentions", mentions, "non_mention_chars", nonMention)
	}

	return ""
}

// ── KeywordFilter ──────────────────────────────────────────────────────────

type KeywordFilter struct {
	keywords []string
}

func (f *KeywordFilter) Check(content, actor string, r *http.Request) string {
	if content == "" {
		return ""
	}
	lower := strings.ToLower(content)
	for _, kw := range f.keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return filters.Reason("keyword", "match", kw)
		}
	}
	return ""
}

// ── DomainFilter ────────────────────────────────────────────────────────────

type DomainFilter struct {
	domains []string
}

func (f *DomainFilter) Check(content, actor string, r *http.Request) string {
	// Check actor domain
	for _, d := range f.domains {
		if strings.Contains(actor, d) {
			return filters.Reason("domain", "domain", d, "actor", actor)
		}
	}
	// Check content for links to blocked domains
	if content != "" {
		lower := strings.ToLower(content)
		for _, d := range f.domains {
			if strings.Contains(lower, d) {
				return filters.Reason("domain_in_content", "domain", d)
			}
		}
	}
	return ""
}
