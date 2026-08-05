package main

import (
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
		"content": "<p><span class=\"h-card\"><a href=\"https://other.example.com/@bob\">@bob</a></span> hello!</p>",
		"tag": [
			{"type": "Mention", "href": "https://other.example.com/@bob", "name": "@bob@other.example.com"}
		]
	}
}`

// misskeyCreateNote is a Misskey-style Create activity with HTML mention content.
const misskeyCreateNote = `{
	"type": "Create",
	"actor": "https://devmi1.example.com/users/user0001",
	"object": {
		"type": "Note",
		"content": "<a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> test",
		"tag": [
			{"type": "Mention", "href": "https://local.example.com/@alice", "name": "@alice@local.example.com"}
		]
	}
}`

// misskeyNote2 is the second test note — HTML mentions with numeric users.
const misskeyNote2 = `{
	"type": "Create",
	"actor": "https://devmi1.example.com/users/user0001",
	"object": {
		"type": "Note",
		"content": "<a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://local.example.com/@1\" class=\"u-url mention\">@1@local.example.com</a> <a href=\"https://local.example.com/@2\" class=\"u-url mention\">@2@local.example.com</a> <a href=\"https://local.example.com/@3\" class=\"u-url mention\">@3@local.example.com</a> <a href=\"https://local.example.com/@4\" class=\"u-url mention\">@4@local.example.com</a> <a href=\"https://local.example.com/@5\" class=\"u-url mention\">@5@local.example.com</a>",
		"tag": [
			{"type": "Mention", "href": "https://local.example.com/@alice", "name": "@alice@local.example.com"}
		]
	}
}`

// misskeySpamNote is a Misskey-style spam note with many mentions and no real content.
const misskeySpamNote = `{
	"type": "Create",
	"actor": "https://spam.example.com/users/bot",
	"object": {
		"type": "Note",
		"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com @f@x.example.com @g@x.example.com spam spam",
		"tag": [
			{"type": "Mention", "href": "https://x.example.com/@a", "name": "@a@x.example.com"},
			{"type": "Mention", "href": "https://x.example.com/@b", "name": "@b@x.example.com"},
			{"type": "Mention", "href": "https://x.example.com/@c", "name": "@c@x.example.com"},
			{"type": "Mention", "href": "https://x.example.com/@d", "name": "@d@x.example.com"},
			{"type": "Mention", "href": "https://x.example.com/@e", "name": "@e@x.example.com"},
			{"type": "Mention", "href": "https://x.example.com/@f", "name": "@f@x.example.com"},
			{"type": "Mention", "href": "https://x.example.com/@g", "name": "@g@x.example.com"}
		]
	}
}`

// misskeyNoteWithKeyword is a Misskey note containing a blocked keyword.
const misskeyNoteWithKeyword = `{
	"type": "Create",
	"actor": "https://dev.example.com/users/spammer",
	"object": {
		"type": "Note",
		"content": "check out spam.example.com for free stuff!"
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
		{`<a href="https://x.example.com/@a" class="u-url mention">@a@x.example.com</a>`, 1},
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
		{"@a@x.example.com @b@x.example.com @c@x.example.com", 3},
		// 6 mentions separated by spaces (Misskey format)
		{"@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com @f@x.example.com test", 6},
		// Malformed double-mention
		{"@foo@bar@baz.example.com", 1},
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
	spam := "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com"
	if r := f.Check(spam, "", nil); r == "" {
		t.Error("should block plain text mention spam")
	}
}

func TestMentionFilter_APTags(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9}

	// The note from the URL has 1 AP Mention tag but 6 plain-text mentions.
	// Content detects 6 mentions > max 4 → should block.
	info := parsePayload([]byte(misskeyCreateNote))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)

	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Errorf("note with 6 content mentions should block (AP tags=%d)", info.APMentions)
	}

	// Spam note with 7 AP Mention tags should be blocked.
	info2 := parsePayload([]byte(misskeySpamNote))
	r2, _ := http.NewRequest("POST", "/inbox", nil)
	r2 = withPayloadInfo(r2, info2)

	if reason := f.Check(info2.Content, "", r2); reason == "" {
		t.Errorf("note with %d AP tags should be blocked", info2.APMentions)
	}
}

// ── Test: KeywordFilter ──────────────────────────────────────────────────────

func TestKeywordFilter(t *testing.T) {
	f := &KeywordFilter{keywords: []string{"spam.example.com", "spam-url.example.com"}}

	if r := f.Check("hello world", "", nil); r != "" {
		t.Errorf("should not block normal: %s", r)
	}
	if r := f.Check("visit spam.example.com now!", "", nil); r == "" {
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

func TestKeywordFilter_MultiByte(t *testing.T) {
	f := &KeywordFilter{keywords: []string{"スパム", "広告"}}

	// Japanese text containing blocked keyword
	if r := f.Check("これはスパムです", "", nil); r == "" {
		t.Error("should block Japanese keyword スパム")
	}
	// Japanese text without blocked keyword
	if r := f.Check("こんにちは世界", "", nil); r != "" {
		t.Errorf("should not block normal Japanese text: %s", r)
	}
	// Keyword split by comma in env var should work
	if r := f.Check("最新の広告をご覧ください", "", nil); r == "" {
		t.Error("should block Japanese keyword 広告")
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
	info := parsePayload(body)
	if info.Content != `<p><span class="h-card"><a href="https://other.example.com/@bob">@bob</a></span> hello!</p>` {
		t.Errorf("unexpected content: %q", info.Content)
	}
	if info.Actor != "https://mastodon.example.com/users/alice" {
		t.Errorf("unexpected actor: %q", info.Actor)
	}
	if info.APMentions != 1 {
		t.Errorf("expected 1 mention from tags, got %d", info.APMentions)
	}
	if info.ActType != "Create" {
		t.Errorf("expected activity type 'Create', got %q", info.ActType)
	}

	// Direct Note (no wrapper)
	body2 := []byte(mastodonDirectNote)
	info2 := parsePayload(body2)
	if info2.Content != `<p>just a normal post</p>` {
		t.Errorf("unexpected direct content: %q", info2.Content)
	}
	if info2.ActType != "Note" {
		t.Errorf("expected activity type 'Note', got %q", info2.ActType)
	}
	if info2.APMentions != 0 {
		t.Errorf("expected 0 mentions, got %d", info2.APMentions)
	}

	// Misskey note
	body3 := []byte(misskeyCreateNote)
	info3 := parsePayload(body3)
	if info3.Actor != "https://devmi1.example.com/users/user0001" {
		t.Errorf("unexpected actor: %q", info3.Actor)
	}
	if info3.APMentions != 1 {
		t.Errorf("expected 1 mention from tag, got %d", info3.APMentions)
	}
	// Verify content contains the test string
	if !strings.Contains(info3.Content, "test") {
		t.Errorf("content should contain 'test': %q", info3.Content)
	}
}

func TestParsePayload_InReplyToAndRecipients(t *testing.T) {
	// inReplyTo as a string, recipients as arrays of URLs and objects.
	body := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"to": ["https://www.w3.org/ns/activitystreams#Public"],
		"cc": ["https://local.example.com/users/1", {"id": "https://local.example.com/users/2", "type": "Person"}],
		"object": {
			"type": "Note",
			"inReplyTo": "https://local.example.com/@alice/123",
			"content": "hi",
			"tag": [
				{"type": "Mention", "href": "https://local.example.com/@alice", "name": "@alice@local.example.com"}
			]
		}
	}`
	info := parsePayload([]byte(body))
	if info.InReplyTo != "https://local.example.com/@alice/123" {
		t.Errorf("unexpected inReplyTo: %q", info.InReplyTo)
	}
	if len(info.MentionURLs) != 1 || info.MentionURLs[0] != "https://local.example.com/@alice" {
		t.Errorf("unexpected MentionURLs: %v", info.MentionURLs)
	}
	if len(info.MentionNames) != 1 || info.MentionNames[0] != "@alice@local.example.com" {
		t.Errorf("unexpected MentionNames: %v", info.MentionNames)
	}
	if len(info.ToCC) != 3 {
		t.Errorf("expected 3 recipients, got %v", info.ToCC)
	}

	// inReplyTo as an object with id.
	body2 := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"object": {
			"type": "Note",
			"inReplyTo": {"id": "https://local.example.com/@alice/456", "type": "Note"},
			"content": "hi"
		}
	}`
	info2 := parsePayload([]byte(body2))
	if info2.InReplyTo != "https://local.example.com/@alice/456" {
		t.Errorf("unexpected inReplyTo (object form): %q", info2.InReplyTo)
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
		blockKeywords:   []string{"spam.example.com"},
		blockDomains:    []string{"baddomain.example.com"},
	}

	chain := buildFilterChain(cfg)

	if len(chain) != 3 {
		t.Fatalf("expected 3 filters, got %d", len(chain))
	}

	// Normal post should pass
	r, _ := http.NewRequest("POST", "/inbox", nil)
	body := []byte(`{"type":"Create","actor":"https://good.example.com/@alice","object":{"type":"Note","content":"hello world","tag":[]}}`)
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
			reason, blocked, info := chain.CheckVerbose([]byte(tt.body), r)

			contentMentions := countMentions(info.Content)
			t.Logf("content_mentions=%d ap_mentions=%d content=%q",
				contentMentions, info.APMentions, info.Content)

			if !blocked {
				t.Errorf("should be blocked: content_mentions=%d ap_mentions=%d",
					contentMentions, info.APMentions)
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

// ── Test: target detection helpers ──────────────────────────────────────────

func TestHostMatches(t *testing.T) {
	cases := []struct {
		raw    string
		domain string
		want   bool
	}{
		{"https://local.example.com/@alice", "local.example.com", true},
		{"https://local.example.com:8443/@alice", "local.example.com", true},
		{"https://LOCAL.EXAMPLE.COM/@alice", "local.example.com", true},
		{"acct:user@local.example.com", "local.example.com", true},
		{"https://local.example.com.evil.example.com/@x", "local.example.com", false},
		{"https://evil.example.com/local.example.com", "local.example.com", false},
		{"", "local.example.com", false},
		{"not a url", "local.example.com", false},
	}

	for _, c := range cases {
		if got := hostMatches(c.raw, c.domain); got != c.want {
			t.Errorf("hostMatches(%q, %q) = %v, want %v", c.raw, c.domain, got, c.want)
		}
	}
}

func TestAcctMatches(t *testing.T) {
	cases := []struct {
		acct   string
		domain string
		want   bool
	}{
		{"@alice@local.example.com", "local.example.com", true},
		{"alice@local.example.com", "local.example.com", true},
		{"@alice@local.example.com", "LOCAL.EXAMPLE.COM", true},
		{"@x@local.example.com.evil.example.com", "local.example.com", false},
		{"@x@other.example.com", "local.example.com", false},
		{"@local", "local.example.com", false},
	}

	for _, c := range cases {
		if got := acctMatches(c.acct, c.domain); got != c.want {
			t.Errorf("acctMatches(%q, %q) = %v, want %v", c.acct, c.domain, got, c.want)
		}
	}
}

func TestMentionHrefs(t *testing.T) {
	cases := []struct {
		content string
		want    []string
	}{
		{`<a href="https://a.example.com/@x" class="u-url mention">@x@a.example.com</a>`, []string{"https://a.example.com/@x"}},
		{`<span class="h-card"><a href="https://a.example.com/@x" class="u-url mention">@x</a></span>`, []string{"https://a.example.com/@x"}},
		{`<a class="mention" href="https://b.example.com/@y">@y</a>`, []string{"https://b.example.com/@y"}},
		{`<a href="https://a.example.com/blog" class="u-url">link</a>`, nil},
		{`<a href="https://a.example.com/@x">@x</a>`, nil},
		{`hello world`, nil},
	}

	for _, c := range cases {
		got := mentionHrefs(c.content)
		if len(got) != len(c.want) {
			t.Errorf("mentionHrefs(%q) = %v, want %v", c.content, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("mentionHrefs(%q) = %v, want %v", c.content, got, c.want)
				break
			}
		}
	}
}

func TestPlainMentionMatches(t *testing.T) {
	cases := []struct {
		content string
		domain  string
		want    bool
	}{
		{"@a@x.example.com @b@local.example.com hi", "local.example.com", true},
		{"@a@x.example.com @b@x.example.com hi", "local.example.com", false},
		{"hello world", "local.example.com", false},
		{"contact user@example.com", "local.example.com", false},
	}

	for _, c := range cases {
		if got := plainMentionMatches(c.content, c.domain); got != c.want {
			t.Errorf("plainMentionMatches(%q, %q) = %v, want %v", c.content, c.domain, got, c.want)
		}
	}
}

// ── Test: MentionFilter targeting ───────────────────────────────────────────

// localMentionSpam is spam that targets the local account via an AP tag.
const localMentionSpam = `{
	"type": "Create",
	"actor": "https://remote.example.com/@spammer",
	"to": ["https://www.w3.org/ns/activitystreams#Public"],
	"cc": ["https://remote.example.com/users/spammer/followers"],
	"object": {
		"type": "Note",
		"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com hi",
		"tag": [
			{"type": "Mention", "href": "https://local.example.com/@alice", "name": "@alice@local.example.com"}
		]
	}
}`

// remoteMentionSpam is spam that mentions only remote accounts.
const remoteMentionSpam = `{
	"type": "Create",
	"actor": "https://remote.example.com/@spammer",
	"object": {
		"type": "Note",
		"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com hi",
		"tag": [
			{"type": "Mention", "href": "https://x.example.com/@a", "name": "@a@x.example.com"}
		]
	}
}`

func TestMentionFilter_TargetMentioned(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9, targetMode: targetMentioned, localDomain: "local.example.com"}

	// Mentions the local account → mention filter applies → blocked.
	info := parsePayload([]byte(localMentionSpam))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)
	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Error("spam mentioning the local domain should be blocked")
	}

	// Mentions only remote accounts → target condition not met → allow.
	info2 := parsePayload([]byte(remoteMentionSpam))
	r2, _ := http.NewRequest("POST", "/inbox", nil)
	r2 = withPayloadInfo(r2, info2)
	if reason := f.Check(info2.Content, "", r2); reason != "" {
		t.Errorf("spam not mentioning the local domain should pass in 'mentioned' mode: %s", reason)
	}
}

func TestMentionFilter_TargetMentioned_HTML(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9, targetMode: targetMentioned, localDomain: "local.example.com"}

	// Misskey-style HTML mention targeting the local domain (no AP tag).
	body := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"object": {
			"type": "Note",
			"content": "<a href=\"https://local.example.com/@alice\" class=\"u-url mention\">@alice@local.example.com</a> <a href=\"https://x.example.com/@a\" class=\"u-url mention\">@a@x.example.com</a> <a href=\"https://x.example.com/@b\" class=\"u-url mention\">@b@x.example.com</a> <a href=\"https://x.example.com/@c\" class=\"u-url mention\">@c@x.example.com</a> <a href=\"https://x.example.com/@d\" class=\"u-url mention\">@d@x.example.com</a> <a href=\"https://x.example.com/@e\" class=\"u-url mention\">@e@x.example.com</a>"
		}
	}`
	info := parsePayload([]byte(body))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)
	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Error("HTML spam mentioning the local domain should be blocked")
	}
}

func TestMentionFilter_TargetMentioned_ToCC(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9, targetMode: targetMentioned, localDomain: "local.example.com"}

	// No local mention in tags/content, but the local actor is in cc → applies.
	body := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"to": ["https://www.w3.org/ns/activitystreams#Public"],
		"cc": ["https://local.example.com/users/1"],
		"object": {
			"type": "Note",
			"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com hi"
		}
	}`
	info := parsePayload([]byte(body))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)
	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Error("spam addressed to a local actor via cc should be blocked")
	}
}

func TestMentionFilter_TargetInReplyTo(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9, targetMode: targetInReplyTo, localDomain: "local.example.com"}

	// inReplyTo points to a local post → filter applies → blocked.
	body := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"object": {
			"type": "Note",
			"inReplyTo": "https://local.example.com/@alice/123",
			"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com hi"
		}
	}`
	info := parsePayload([]byte(body))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)
	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Error("spam replying to a local post should be blocked")
	}

	// inReplyTo points to a remote post → target condition not met → allow.
	body2 := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"object": {
			"type": "Note",
			"inReplyTo": "https://x.example.com/@y/123",
			"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com hi"
		}
	}`
	info2 := parsePayload([]byte(body2))
	r2, _ := http.NewRequest("POST", "/inbox", nil)
	r2 = withPayloadInfo(r2, info2)
	if reason := f.Check(info2.Content, "", r2); reason != "" {
		t.Errorf("spam replying to a remote post should pass in 'in_reply_to' mode: %s", reason)
	}

	// No inReplyTo at all → target condition not met → allow.
	info3 := parsePayload([]byte(remoteMentionSpam))
	r3, _ := http.NewRequest("POST", "/inbox", nil)
	r3 = withPayloadInfo(r3, info3)
	if reason := f.Check(info3.Content, "", r3); reason != "" {
		t.Errorf("spam with no inReplyTo should pass in 'in_reply_to' mode: %s", reason)
	}
}

func TestMentionFilter_TargetMentionedOrInReplyTo(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9, targetMode: targetMentionedOrInReplyTo, localDomain: "local.example.com"}

	// Matches via inReplyTo even though no local mention.
	body := `{
		"type": "Create",
		"actor": "https://remote.example.com/@x",
		"object": {
			"type": "Note",
			"inReplyTo": "https://local.example.com/@alice/123",
			"content": "@a@x.example.com @b@x.example.com @c@x.example.com @d@x.example.com @e@x.example.com hi"
		}
	}`
	info := parsePayload([]byte(body))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)
	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Error("spam replying to a local post should be blocked in combined mode")
	}

	// Neither → allow.
	info2 := parsePayload([]byte(remoteMentionSpam))
	r2, _ := http.NewRequest("POST", "/inbox", nil)
	r2 = withPayloadInfo(r2, info2)
	if reason := f.Check(info2.Content, "", r2); reason != "" {
		t.Errorf("spam with no local targeting should pass: %s", reason)
	}
}

// TestMentionFilter_TargetWithoutLocalDomain verifies the safe fallback:
// a target mode without LOCAL_DOMAIN behaves like "always".
func TestMentionFilter_TargetWithoutLocalDomain(t *testing.T) {
	f := &MentionFilter{maxMentions: 4, maxRatio: 0.9, targetMode: targetMentioned}

	info := parsePayload([]byte(remoteMentionSpam))
	r, _ := http.NewRequest("POST", "/inbox", nil)
	r = withPayloadInfo(r, info)
	if reason := f.Check(info.Content, "", r); reason == "" {
		t.Error("target mode without LOCAL_DOMAIN should fall back to 'always' behavior")
	}
}
