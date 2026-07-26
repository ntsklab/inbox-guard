package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ntsklab/inbox-guard/filters"
)

type activityTag struct {
	Type string `json:"type"`
	HRef string `json:"href"`
	Name string `json:"name"`
}

// activityType handles the "type" field which can be a string or []string.
type activityType string

func (t *activityType) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*t = activityType(s)
		return nil
	}
	var arr []string
	if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
		*t = activityType(arr[0])
		return nil
	}
	return nil
}

// tagList handles the "tag" field which can be a single object or an array.
type tagList []activityTag

func (tl *tagList) UnmarshalJSON(data []byte) error {
	var arr []activityTag
	if json.Unmarshal(data, &arr) == nil {
		*tl = arr
		return nil
	}
	var single activityTag
	if json.Unmarshal(data, &single) == nil {
		*tl = []activityTag{single}
		return nil
	}
	return nil
}

type activityObject struct {
	Type   activityType    `json:"type"`
	Actor  json.RawMessage `json:"actor"`
	Object json.RawMessage `json:"object"`
}

// payloadDetail holds the content and tags extracted from an activity's object.
type payloadDetail struct {
	Content string  `json:"content"`
	Name    string  `json:"name"`
	Tag     tagList `json:"tag"`
}

func extractActor(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.ID
	}
	return ""
}

func parsePayload(body []byte) (content, actor, actType string, apMentions int) {
	var act activityObject
	if err := json.Unmarshal(body, &act); err != nil {
		return "", "", "", 0
	}

	actType = string(act.Type)
	actor = extractActor(act.Actor)

	// Object can be a string (URL), an object, or an array of either.
	var detail payloadDetail
	if len(act.Object) > 0 {
		switch act.Object[0] {
		case '{':
			json.Unmarshal(act.Object, &detail)
		case '[':
			// Array: use the first element that is an object with content.
			var arr []json.RawMessage
			if json.Unmarshal(act.Object, &arr) == nil {
				for _, elem := range arr {
					if len(elem) > 0 && elem[0] == '{' {
						if json.Unmarshal(elem, &detail) == nil && detail.Content != "" {
							break
						}
					}
				}
			}
		}
		// String (URL): no content to extract.
	}

	content = detail.Content
	if content == "" {
		content = detail.Name
	}
	for _, t := range detail.Tag {
		if t.Type == "Mention" {
			apMentions++
		}
	}

	// Fallback: the body itself may be a direct object (Note without Create wrapper).
	if content == "" {
		var d payloadDetail
		json.Unmarshal(body, &d)
		content = d.Content
		if content == "" {
			content = d.Name
		}
		if apMentions == 0 {
			for _, t := range d.Tag {
				if t.Type == "Mention" {
					apMentions++
				}
			}
		}
	}

	return content, actor, actType, apMentions
}

// getContent is a convenience wrapper for callers that only need content and actor.
func getContent(body []byte) (string, string) {
	c, a, _, _ := parsePayload(body)
	return c, a
}

// countMentions counts mentions in content.
// Supports Mastodon (class="h-card") and Misskey (class="mention") HTML formats,
// as well as plain text @user@domain patterns.
func countMentions(content string) int {
	// Detect HTML mention patterns: Mastodon uses h-card, Misskey uses mention.
	if n := strings.Count(content, `class="h-card"`); n > 0 {
		return n
	}
	if n := strings.Count(content, `class="mention"`); n > 0 {
		return n
	}
	if n := strings.Count(content, `class="u-url mention"`); n > 0 {
		return n
	}
	// Fall back to plain text mention counting.
	return countPlainMentions(content)
}

// countPlainMentions counts @user@domain patterns in plain text content.
func countPlainMentions(content string) int {
	count := 0
	for _, word := range strings.Fields(content) {
		if strings.Count(word, "@") >= 2 && strings.HasPrefix(word, "@") {
			count++
		}
	}
	return count
}

// nonMentionContent estimates non-mention text length by stripping HTML tags.
func nonMentionContent(content string) int {
	text := stripTags(content)
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

type contextKey string

const apMentionsKey contextKey = "ap_mentions"

// Check runs all filters in sequence. Returns the first blocking reason.
func (fc filterChain) Check(body []byte, r *http.Request) (reason string, blocked bool) {
	if len(fc) == 0 {
		return "", false
	}

	content, actor, _, apMentions := parsePayload(body)

	if apMentions > 0 {
		ctx := context.WithValue(r.Context(), apMentionsKey, apMentions)
		r = r.WithContext(ctx)
	}

	for _, f := range fc {
		if reason := f.Check(content, actor, r); reason != "" {
			return reason, true
		}
	}

	return "", false
}

// CheckVerbose runs all filters and returns diagnostic info.
func (fc filterChain) CheckVerbose(body []byte, r *http.Request) (reason string, blocked bool, content, actor, actType string, apMentions int) {
	content, actor, actType, apMentions = parsePayload(body)

	if len(fc) == 0 {
		return "", false, content, actor, actType, apMentions
	}

	if apMentions > 0 {
		ctx := context.WithValue(r.Context(), apMentionsKey, apMentions)
		r = r.WithContext(ctx)
	}

	for _, f := range fc {
		if reason := f.Check(content, actor, r); reason != "" {
			return reason, true, content, actor, actType, apMentions
		}
	}

	return "", false, content, actor, actType, apMentions
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

	// Use whichever is higher: content-based count or AP tag count.
	mentions := countMentions(content)
	if r != nil {
		if apCount, ok := r.Context().Value(apMentionsKey).(int); ok && apCount > mentions {
			mentions = apCount
		}
	}

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
	for _, d := range f.domains {
		if strings.Contains(actor, d) {
			return filters.Reason("domain", "domain", d, "actor", actor)
		}
	}
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
