package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
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
	To     json.RawMessage `json:"to"`
	CC     json.RawMessage `json:"cc"`
}

// payloadDetail holds the fields extracted from an activity's object.
type payloadDetail struct {
	Content   string          `json:"content"`
	Name      string          `json:"name"`
	Tag       tagList         `json:"tag"`
	InReplyTo json.RawMessage `json:"inReplyTo"`
	To        json.RawMessage `json:"to"`
	CC        json.RawMessage `json:"cc"`
}

// payloadInfo is the fully parsed activity payload shared with filters via
// the request context.
type payloadInfo struct {
	Content      string
	Actor        string
	ActType      string
	APMentions   int
	MentionURLs  []string // href of Mention tags
	MentionNames []string // name (acct) of Mention tags
	InReplyTo    string
	ToCC         []string // normalized recipient URLs from to/cc
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

// normalizeID resolves a single activity field that may be a string or an
// object with an "id".
func normalizeID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.ID
	}
	return ""
}

// parseRecipients normalizes a to/cc field, which may be an array of strings
// or objects, or a single value.
func parseRecipients(raw json.RawMessage) []string {
	var out []string
	if len(raw) == 0 {
		return out
	}
	if raw[0] == '[' {
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil {
			for _, e := range arr {
				if id := normalizeID(e); id != "" {
					out = append(out, id)
				}
			}
		}
		return out
	}
	if id := normalizeID(raw); id != "" {
		out = append(out, id)
	}
	return out
}

// collectMentions records AP Mention tags and their targets.
func collectMentions(tags tagList, info *payloadInfo) {
	for _, t := range tags {
		if t.Type != "Mention" {
			continue
		}
		info.APMentions++
		if t.HRef != "" {
			info.MentionURLs = append(info.MentionURLs, t.HRef)
		}
		if t.Name != "" {
			info.MentionNames = append(info.MentionNames, t.Name)
		}
	}
}

func parsePayload(body []byte) payloadInfo {
	var act activityObject
	if err := json.Unmarshal(body, &act); err != nil {
		return payloadInfo{}
	}

	info := payloadInfo{
		ActType: string(act.Type),
		Actor:   extractActor(act.Actor),
	}

	info.ToCC = append(info.ToCC, parseRecipients(act.To)...)
	info.ToCC = append(info.ToCC, parseRecipients(act.CC)...)

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

	info.Content = detail.Content
	if info.Content == "" {
		info.Content = detail.Name
	}
	info.InReplyTo = normalizeID(detail.InReplyTo)
	info.ToCC = append(info.ToCC, parseRecipients(detail.To)...)
	info.ToCC = append(info.ToCC, parseRecipients(detail.CC)...)
	collectMentions(detail.Tag, &info)

	// Fallback: the body itself may be a direct object (Note without Create wrapper).
	if info.Content == "" {
		var d payloadDetail
		json.Unmarshal(body, &d)
		info.Content = d.Content
		if info.Content == "" {
			info.Content = d.Name
		}
		if info.InReplyTo == "" {
			info.InReplyTo = normalizeID(d.InReplyTo)
		}
		if info.APMentions == 0 {
			collectMentions(d.Tag, &info)
		}
		info.ToCC = append(info.ToCC, parseRecipients(d.To)...)
		info.ToCC = append(info.ToCC, parseRecipients(d.CC)...)
	}

	return info
}

// getContent is a convenience wrapper for callers that only need content and actor.
func getContent(body []byte) (string, string) {
	info := parsePayload(body)
	return info.Content, info.Actor
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
		chain = append(chain, &MentionFilter{
			maxMentions: cfg.maxMentions,
			maxRatio:    cfg.maxContentRatio,
			targetMode:  cfg.mentionTarget,
			localDomain: cfg.localDomain,
		})
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

const payloadKey contextKey = "payload"

// withPayloadInfo attaches the parsed payload to the request context.
func withPayloadInfo(r *http.Request, info payloadInfo) *http.Request {
	ctx := context.WithValue(r.Context(), payloadKey, info)
	return r.WithContext(ctx)
}

// Check runs all filters in sequence. Returns the first blocking reason.
func (fc filterChain) Check(body []byte, r *http.Request) (reason string, blocked bool) {
	if len(fc) == 0 {
		return "", false
	}

	info := parsePayload(body)
	r = withPayloadInfo(r, info)

	for _, f := range fc {
		if reason := f.Check(info.Content, info.Actor, r); reason != "" {
			return reason, true
		}
	}

	return "", false
}

// CheckVerbose runs all filters and returns diagnostic info.
func (fc filterChain) CheckVerbose(body []byte, r *http.Request) (reason string, blocked bool, info payloadInfo) {
	info = parsePayload(body)
	r = withPayloadInfo(r, info)

	if len(fc) == 0 {
		return "", false, info
	}

	for _, f := range fc {
		if reason := f.Check(info.Content, info.Actor, r); reason != "" {
			return reason, true, info
		}
	}

	return "", false, info
}

// ── MentionFilter ──────────────────────────────────────────────────────────

type MentionFilter struct {
	maxMentions int
	maxRatio    float64
	targetMode  string
	localDomain string
}

func (f *MentionFilter) Check(content, actor string, r *http.Request) string {
	if content == "" {
		return ""
	}

	if !f.appliesTo(r, content) {
		return ""
	}

	// Use whichever is higher: content-based count or AP tag count.
	mentions := countMentions(content)
	if r != nil {
		if info, ok := r.Context().Value(payloadKey).(payloadInfo); ok && info.APMentions > mentions {
			mentions = info.APMentions
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

// appliesTo reports whether the mention filter should be enforced for this
// activity based on the configured target mode. Without a target mode or
// local domain it falls back to the legacy "always" behavior.
func (f *MentionFilter) appliesTo(r *http.Request, content string) bool {
	if f.targetMode == "" || f.targetMode == targetAlways || f.localDomain == "" {
		return true
	}

	var info payloadInfo
	if r != nil {
		info, _ = r.Context().Value(payloadKey).(payloadInfo)
	}

	switch f.targetMode {
	case targetMentioned:
		return f.mentionedLocally(info, content)
	case targetInReplyTo:
		return f.inReplyToLocally(info)
	case targetMentionedOrInReplyTo:
		return f.mentionedLocally(info, content) || f.inReplyToLocally(info)
	}
	return true
}

// mentionedLocally reports whether any mention of the payload targets the
// local domain, across AP tags, to/cc recipients, HTML content, and plain text.
func (f *MentionFilter) mentionedLocally(info payloadInfo, content string) bool {
	for _, u := range info.MentionURLs {
		if hostMatches(u, f.localDomain) {
			return true
		}
	}
	for _, n := range info.MentionNames {
		if acctMatches(n, f.localDomain) {
			return true
		}
	}
	for _, u := range info.ToCC {
		if hostMatches(u, f.localDomain) {
			return true
		}
	}
	for _, h := range mentionHrefs(content) {
		if hostMatches(h, f.localDomain) {
			return true
		}
	}
	return plainMentionMatches(content, f.localDomain)
}

// inReplyToLocally reports whether the payload is a reply to a local post.
func (f *MentionFilter) inReplyToLocally(info payloadInfo) bool {
	return info.InReplyTo != "" && hostMatches(info.InReplyTo, f.localDomain)
}

// hostMatches reports whether raw is a URL (or acct URI) whose host equals
// the given domain. Uses exact host equality to avoid substring bypasses.
func hostMatches(raw, domain string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" && u.Scheme != "" {
		// Opaque URI such as acct:user@domain.
		if i := strings.LastIndex(raw, "@"); i >= 0 {
			host = raw[i+1:]
		}
	}
	if host == "" {
		return false
	}
	return strings.EqualFold(host, domain)
}

// acctMatches reports whether an acct string (@user@domain or user@domain)
// belongs to the given domain.
func acctMatches(acct, domain string) bool {
	acct = strings.TrimPrefix(acct, "@")
	if i := strings.LastIndex(acct, "@"); i >= 0 {
		return strings.EqualFold(acct[i+1:], domain)
	}
	return false
}

// plainMentionMatches reports whether content contains a plain text
// @user@domain mention targeting the given domain.
func plainMentionMatches(content, domain string) bool {
	for _, word := range strings.Fields(content) {
		if strings.HasPrefix(word, "@") && strings.Count(word, "@") >= 2 && acctMatches(word, domain) {
			return true
		}
	}
	return false
}

var (
	anchorTagRe = regexp.MustCompile(`(?i)<a\b[^>]*>`)
	attrRe      = regexp.MustCompile(`([a-zA-Z-]+)\s*=\s*"([^"]*)"`)
)

// mentionHrefs extracts the href of every <a> tag whose class contains
// "mention" (Mastodon: "u-url mention", Misskey: "mention").
func mentionHrefs(content string) []string {
	var hrefs []string
	for _, tag := range anchorTagRe.FindAllString(content, -1) {
		attrs := make(map[string]string)
		for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(m[1])] = m[2]
		}
		if !strings.Contains(attrs["class"], "mention") {
			continue
		}
		if href := attrs["href"]; href != "" {
			hrefs = append(hrefs, href)
		}
	}
	return hrefs
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
