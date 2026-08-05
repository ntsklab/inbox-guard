package main

import "testing"

func TestLoadConfig_Defaults(t *testing.T) {
	cfg := loadConfig()

	if cfg.mentionTarget != targetAlways {
		t.Errorf("mentionTarget default = %q, want %q", cfg.mentionTarget, targetAlways)
	}
	if cfg.localDomain != "" {
		t.Errorf("localDomain default = %q, want empty", cfg.localDomain)
	}
}

func TestLoadConfig_MentionFilterTarget(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", targetAlways},
		{"mentioned", targetMentioned},
		{"in_reply_to", targetInReplyTo},
		{"mentioned_or_in_reply_to", targetMentionedOrInReplyTo},
		{"bogus", targetAlways},
	}

	for _, c := range cases {
		t.Setenv("MENTION_FILTER_TARGET", c.env)
		cfg := loadConfig()
		if cfg.mentionTarget != c.want {
			t.Errorf("MENTION_FILTER_TARGET=%q → mentionTarget=%q, want %q", c.env, cfg.mentionTarget, c.want)
		}
	}
}

func TestLoadConfig_LocalDomain(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", ""},
		{"hl.oyasumi.dev", "hl.oyasumi.dev"},
		{"https://hl.oyasumi.dev/", "hl.oyasumi.dev"},
		{"HL.OYASUMI.DEV", "hl.oyasumi.dev"},
		{"hl.oyasumi.dev:8443", "hl.oyasumi.dev"},
		{"  ", ""},
	}

	for _, c := range cases {
		t.Setenv("LOCAL_DOMAIN", c.env)
		cfg := loadConfig()
		if cfg.localDomain != c.want {
			t.Errorf("LOCAL_DOMAIN=%q → localDomain=%q, want %q", c.env, cfg.localDomain, c.want)
		}
	}
}
