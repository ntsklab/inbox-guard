package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestCountMentions(t *testing.T) {
	cases := []struct {
		content  string
		expected int
	}{
		{"", 0},
		{`<span class="h-card">@user</span>`, 1},
		{strings.Repeat(`<span class="h-card">@x</span>`, 210), 210},
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

func TestMentionFilter(t *testing.T) {
	f := &MentionFilter{maxMentions: 50, maxRatio: 0.9}

	// Under limit
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

func TestKeywordFilter(t *testing.T) {
	f := &KeywordFilter{keywords: []string{"ctkpaarr.org", "spam-url.com"}}

	if r := f.Check("hello world", "", nil); r != "" {
		t.Errorf("should not block normal: %s", r)
	}
	if r := f.Check("visit ctkpaarr.org now!", "", nil); r == "" {
		t.Error("should block keyword")
	}
}

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

func TestGetContent(t *testing.T) {
	// Create activity wrapping a Note
	body := `{"type":"Create","actor":"https://example.com/users/1","object":{"type":"Note","content":"hello world"}}`
	content, actor := getContent([]byte(body))
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
	if actor != "https://example.com/users/1" {
		t.Errorf("expected actor URL, got %q", actor)
	}

	// Direct Note (no wrapper)
	body2 := `{"type":"Note","content":"direct note"}`
	content2, _ := getContent([]byte(body2))
	if content2 != "direct note" {
		t.Errorf("expected 'direct note', got %q", content2)
	}

	// Actor as object with id field
	body3 := `{"type":"Create","actor":{"id":"https://example.com/users/2","type":"Person"},"object":{"type":"Note","content":"hello"}}`
	content3, actor3 := getContent([]byte(body3))
	if content3 != "hello" {
		t.Errorf("expected 'hello', got %q", content3)
	}
	if actor3 != "https://example.com/users/2" {
		t.Errorf("expected actor URL from object, got %q", actor3)
	}
}
