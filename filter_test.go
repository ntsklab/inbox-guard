package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// ── Test fixtures: real ActivityPub payloads ──────────────────────────────────

// mastodonCreateNote is a Mastodon-style Create activity with HTML content.
const mastodonCreateNote = `{
	"type": "Create",
	"actor": "https://mastodon.example.com/users/alice",
	"object": {
		"type": "Note",
		"content": "<p><span class=\"h-card\"><a href=\"https://other.example/@bob\">@bob</a></span> hello!</p>",
		"tag": [
			{"type": "Mention", "href": "https://other.example/@bob", "name": "@bob@other.example"}
		]
	}
}`

// misskeyCreateNote is a Misskey-style Create activity with HTML mention content.
const misskeyCreateNote = `{
	"type": "Create",
	"actor": "https://devmi1.oyasumi.dev/users/9wnub9apt58g0001",
	"object": {
		"type": "Note",
		"content": "<a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> test",
		"tag": [
			{"type": "Mention", "href": "https://hl.oyasumi.dev/@ntek", "name": "@ntek@hl.oyasumi.dev"}
		]
	}
}`

// misskeyNote2 is the second test note — HTML mentions with numeric users.
const misskeyNote2 = `{
	"type": "Create",
	"actor": "https://devmi1.oyasumi.dev/users/9wnub9apt58g0001",
	"object": {
		"type": "Note",
		"content": "<a href=\"https://hl.oyasumi.dev/@ntek\" class=\"u-url mention\">@ntek@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@1\" class=\"u-url mention\">@1@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@2\" class=\"u-url mention\">@2@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@3\" class=\"u-url mention\">@3@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@4\" class=\"u-url mention\">@4@hl.oyasumi.dev</a> <a href=\"https://hl.oyasumi.dev/@5\" class=\"u-url mention\">@5@hl.oyasumi.dev</a>",
		"tag": [
			{"type": "Mention", "href": "https://hl.oyasumi.dev/@ntek", "name": "@ntek@hl.oyasumi.dev"}
		]
	}
}`

// misskeySpamNote is a Misskey-style spam note with many mentions and no real content.
const misskeySpamNote = `{
	"type": "Create",
	"actor": "https://spam.example/users/bot",
	"object": {
		"type": "Note",
		"content": "@a@x.com @b@x.com @c@x.com @d@x.com @e@x.com @f@x.com @g@x.com spam spam",
		"tag": [
			{"type": "Mention", "href": "https://x.com/@a", "name": "@a@x.com"},
			{"type": "Mention", "href": "https://x.com/@b", "name": "@b@x.com"},
			{"type": "Mention", "href": "https://x.com/@c", "name": "@c@x.com"},
			{"type": "Mention", "href": "https://x.com/@d", "name": "@d@x.com"},
			{"type": "Mention", "href": "https://x.com/@e", "name": "@e@x.com"},
			{"type": "Mention", "href": "https://x.com/@f", "name": "@f@x.com"},
			{"type": "Mention", "href": "https://x.com/@g", "name": "@g@x.com"}
		]
	}
}`

// misskeyNoteWithKeyword is a Misskey note containing a blocked keyword.
const misskeyNoteWithKeyword = `{
	"type": "Create",
	"actor": "https://dev.example/users/spammer",
	"object": {
		"type": "Note",
		"content": "check out ctkpaarr.org for free stuff!"
	}
}`

// mastodonDirectNote is a Note received directly (not wrapped in Create).
const mastodonDirectNote = `{
	"type": "Note",
	"content": "<p>just a normal post</p>"
}`

// ── Test: countMentions (HTML) ────────────────────────────────────────────────

func TestCountMentions(t *testing.T) {
	cases := []struct {
		content  string
		expected int
	}{
		{"", 0},
		// Mastodon: class="h-card"
		{`<span class="h-card">@user</span>`, 1},
		{strings.Repeat(`<span class="h-card">@x</span>`, 210), 210},
		// Misskey: class="u-url mention"
		{`<a href="https://x.com/@a" class="u-url mention">@a@x.com</a>`, 1},
		{strings.Repeat(`<a href="x" class="u-url mention">@x</a>`, 6), 6},
		// Plain text fallback
		{`hello world`, 0},
	}

	for _, c := range cases {
		got := countMentions(c.content)
		display := c.content
		if len(display) > 50 {
			display = display[:50]
		}
		if got != c.expected {
			t.Errorf("countMentions(%q) = %d, want %d", display, got, c.expected)
		}
	}
}

// ── Test: countMentions (plain text fallback) ─────────────────────────────────

func TestCountPlainMentions(t *testing.T) {
	cases := []struct {
		content  string
		expected int
	}{
		{"", 0},
		{"hello world", 0},
		{"@user@domain", 1},
		{"@user@domain hello", 1},
		{"@a@x.com @b@x.com @c@x.com", 3},
		// 6 mentions separated by spaces (Misskey format)
		{"@a@x.com @b@x.com @c@x.com @d@x.com @e@x.com @f@x.com test", 6},
		// Malformed double-mention
		{"@foo@bar@baz.com", 1},
		// Email should not be counted (single @)
		{"user@example.com", 0},
		{"contact user@example.com for info", 0},
	}

	for _, c := range cases {
		got := countPlainMentions(c.content)
		if got != c.expected {
			t.Errorf("countPlainMentions(%q) = %d, want %d", c.content, got, c.expected)
		}
	}
}

// ── Test: nonMentionContent ──────────────────────────────────────────────────

func TestNonMentionContent(t *testing.T) {
	cases := []struct {
		content  string
		expected int
	}{
		{"", 0},
		{`hello world`, 11},
		{`<span class="h-card"><a href="a">@user</a></span> hello`, 11},
	}

	for _, c := range cases {
		got := nonMentionContent(c.content)
		if got != c.expected {
			t.Errorf("nonMentionContent(%q) = %d, want %d", c.content, got, c.expected)
		}
	}
}

// ── Test: MentionFilter ──────────────────────────────────────────────────────

func TestMentionFilter(t *testing.T) {
	f := &MentionFilter{maxMentions: 50, maxRatio: 0.9}

	// Under limit — HTML format
	content := `<span class="h-card">@user</span> hello world! actual content here`
	if r := f.Check(content, "", nil); r != "" {
		t.Errorf("should not block normal post: %s", r)
	}

	// Over limit
	spamContent := strings.Repeat(`<span class="h-card">@user</span>`, 100)
	if r := f.Check(spamContent, "", nil); r == "" {
		t.Error("should block mention spam")
	}
}

func TestMentionFilter_PlainText(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9}

	// Under limit — plain text (Misskey format)
	content := "@user@domain hello"
	if r := f.Check(content, "", nil); r != "" {
		t.Errorf("should not block normal plain text: %s", r)
	}

	// Over limit — plain text
	spam := "@a@x.com @b@x.com @c@x.com @d@x.com @e@x.com"
	if r := f.Check(spam, "", nil); r == "" {
		t.Error("should block plain text mention spam")
	}
}

func TestMentionFilter_APTags(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9}

	// The note from the URL has 1 AP Mention tag but 6 plain-text mentions.
	// Content detects 6 mentions > max 4 → should block.
	content, _, apMentions := parsePayload([]byte(misskeyCreateNote))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	if apMentions > 0 {
		ctx := context.WithValue(r.Context(), apMentionsKey, apMentions)
		r = r.WithContext(ctx)
	}

	if reason := f.Check(content, "", r); reason == "" {
		t.Errorf("note with 6 content mentions should block (AP tags=%d)", apMentions)
	}

	// Spam note with 7 AP Mention tags should be blocked.
	content2, _, apMentions2 := parsePayload([]byte(misskeySpamNote))
	r2, _ := http.NewRequest("POST", "/inbox", nil)
	if apMentions2 > 0 {
		ctx := context.WithValue(r2.Context(), apMentionsKey, apMentions2)
		r2 = r2.WithContext(ctx)
	}

	if reason := f.Check(content2, "", r2); reason == "" {
		t.Errorf("note with %d AP tags should be blocked", apMentions2)
	}
}

// ── Test: KeywordFilter ──────────────────────────────────────────────────────

func TestKeywordFilter(t *testing.T) {
	f := &KeywordFilter{keywords: []string{"ctkpaarr.org", "spam-url.com"}}

	if r := f.Check("hello world", "", nil); r != "" {
		t.Errorf("should not block normal: %s", r)
	}
	if r := f.Check("visit ctkpaarr.org now!", "", nil); r == "" {
		t.Error("should block keyword")
	}
}

func TestKeywordFilter_CaseInsensitive(t *testing.T) {
	f := &KeywordFilter{keywords: []string{"SPAM", "ViAgRa"}}

	if r := f.Check("buy spam now", "", nil); r == "" {
		t.Error("should block lowercase match of uppercase keyword")
	}
	if r := f.Check("BUY VIAGRA CHEAP", "", nil); r == "" {
		t.Error("should block uppercase match of mixed-case keyword")
	}
}

// ── Test: DomainFilter ───────────────────────────────────────────────────────

func TestDomainFilter(t *testing.T) {
	f := &DomainFilter{domains: []string{"spam.example.com"}}

	if r := f.Check("", "https://good.example.com/users/1", &http.Request{}); r != "" {
		t.Errorf("should not block good domain: %s", r)
	}
	if r := f.Check("", "https://spam.example.com/users/2", &http.Request{}); r == "" {
		t.Error("should block bad domain in actor")
	}
	if r := f.Check("visit spam.example.com today", "", &http.Request{}); r == "" {
		t.Error("should block bad domain in content")
	}
}

// ── Test: parsePayload / getContent ──────────────────────────────────────────

func TestParsePayload(t *testing.T) {
	// Create activity wrapping a Note (Mastodon)
	body := []byte(mastodonCreateNote)
	content, actor, mentions := parsePayload(body)
	if content != `<p><span class="h-card"><a href="https://other.example/@bob">@bob</a></span> hello!</p>` {
		t.Errorf("unexpected content: %q", content)
	}
	if actor != "https://mastodon.example.com/users/alice" {
		t.Errorf("unexpected actor: %q", actor)
	}
	if mentions != 1 {
		t.Errorf("expected 1 mention from tags, got %d", mentions)
	}

	// Direct Note (no wrapper)
	body2 := []byte(mastodonDirectNote)
	content2, _, mentions2 := parsePayload(body2)
	if content2 != `<p>just a normal post</p>` {
		t.Errorf("unexpected direct content: %q", content2)
	}
	if mentions2 != 0 {
		t.Errorf("expected 0 mentions, got %d", mentions2)
	}

	// Misskey note
	body3 := []byte(misskeyCreateNote)
	content3, actor3, mentions3 := parsePayload(body3)
	if actor3 != "https://devmi1.oyasumi.dev/users/9wnub9apt58g0001" {
		t.Errorf("unexpected actor: %q", actor3)
	}
	if mentions3 != 1 {
		t.Errorf("expected 1 mention from tag, got %d", mentions3)
	}
	// Verify content contains the test string
	if !strings.Contains(content3, "test") {
		t.Errorf("content should contain 'test': %q", content3)
	}
}

func TestGetContent(t *testing.T) {
	// Actor as object with id field
	body := `{"type":"Create","actor":{"id":"https://example.com/users/2","type":"Person"},"object":{"type":"Note","content":"hello"}}`
	content, actor := getContent([]byte(body))
	if content != "hello" {
		t.Errorf("expected 'hello', got %q", content)
	}
	if actor != "https://example.com/users/2" {
		t.Errorf("expected actor URL from object, got %q", actor)
	}
}

// ── Test: FilterChain integration ───────────────────────────────────────────

func TestFilterChain_Integration(t *testing.T) {
	cfg := config{
		maxMentions:     4,
		maxContentRatio: 0.9,
		blockKeywords:   []string{"ctkpaarr.org"},
		blockDomains:    []string{"baddomain.com"},
	}

	chain := buildFilterChain(cfg)

	if len(chain) != 3 {
		t.Fatalf("expected 3 filters, got %d", len(chain))
	}

	// Normal post should pass
	r, _ := http.NewRequest("POST", "/inbox", nil)
	body := []byte(`{"type":"Create","actor":"https://good.example/@alice","object":{"type":"Note","content":"hello world","tag":[]}}`)
	reason, blocked := chain.Check(body, r)
	if blocked {
		t.Errorf("normal post should not be blocked: %s", reason)
	}

	// Spam with many mentions should be blocked
	r2, _ := http.NewRequest("POST", "/inbox", nil)
	reason2, blocked2 := chain.Check([]byte(misskeySpamNote), r2)
	if !blocked2 {
		t.Error("spam note with 7 mentions should be blocked")
	}
	if reason2 == "" {
		t.Error("block reason should not be empty")
	}

	// Post with blocked keyword should be blocked
	r3, _ := http.NewRequest("POST", "/inbox", nil)
	reason3, blocked3 := chain.Check([]byte(misskeyNoteWithKeyword), r3)
	if !blocked3 {
		t.Error("post with blocked keyword should be blocked")
	}
	if reason3 == "" {
		t.Error("block reason should not be empty")
	}
}

// Test: exact production notes that should be blocked.
func TestFilterChain_BlockMisskeyNotes(t *testing.T) {
	cfg := config{
		maxMentions: 4,
	}
	chain := buildFilterChain(cfg)

	tests := []struct {
		name string
		body string
	}{
		{"note1 (6 mentions + test)", misskeyCreateNote},
		{"note2 (6 mentions with numeric users)", misskeyNote2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("POST", "/inbox", nil)
			reason, blocked, content, _, apMentions := chain.CheckVerbose([]byte(tt.body), r)

			contentMentions := countMentions(content)
			t.Logf("content_mentions=%d ap_mentions=%d content=%q",
				contentMentions, apMentions, content)

			if !blocked {
				t.Errorf("should be blocked: content_mentions=%d ap_mentions=%d",
					contentMentions, apMentions)
			}
			if reason == "" {
				t.Error("block reason should not be empty")
			}
		})
	}
}

// ── Test: StripTags ──────────────────────────────────────────────────────────

func TestStripTags(t *testing.T) {
	cases := []struct {
		html     string
		expected string
	}{
		{"", ""},
		{"hello", "hello"},
		{"<p>hello</p>", "hello"},
		{"<span class=\"h-card\"><a href=\"x\">@user</a></span> world", "@user world"},
	}

	for _, c := range cases {
		got := stripTags(c.html)
		if got != c.expected {
			t.Errorf("stripTags(%q) = %q, want %q", c.html, got, c.expected)
		}
	}
}
