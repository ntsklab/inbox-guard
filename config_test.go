package main

import "testing"

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() with defaults returned error: %v", err)
	}

	if cfg.mentionTarget != targetAlways {
		t.Errorf("mentionTarget default = %q, want %q", cfg.mentionTarget, targetAlways)
	}
	if cfg.localDomain != "" {
		t.Errorf("localDomain default = %q, want empty", cfg.localDomain)
	}
	if cfg.metricsPort != 9090 {
		t.Errorf("metricsPort default = %d, want 9090", cfg.metricsPort)
	}
}

func TestLoadConfig_MentionFilterTarget(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", targetAlways},
		{"always", targetAlways},
		{"mentioned", targetMentioned},
		{"in_reply_to", targetInReplyTo},
		{"mentioned_or_in_reply_to", targetMentionedOrInReplyTo},
	}

	for _, c := range cases {
		t.Setenv("MENTION_FILTER_TARGET", c.env)
		cfg, err := loadConfig()
		if err != nil {
			t.Errorf("MENTION_FILTER_TARGET=%q returned error: %v", c.env, err)
			continue
		}
		if cfg.mentionTarget != c.want {
			t.Errorf("MENTION_FILTER_TARGET=%q → mentionTarget=%q, want %q", c.env, cfg.mentionTarget, c.want)
		}
	}
}

func TestLoadConfig_InvalidValues(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad port", map[string]string{"LISTEN_PORT": "abc"}},
		{"port zero", map[string]string{"LISTEN_PORT": "0"}},
		{"port overflow", map[string]string{"LISTEN_PORT": "70000"}},
		{"bad metrics port", map[string]string{"METRICS_PORT": "abc"}},
		{"metrics port zero", map[string]string{"METRICS_PORT": "0"}},
		{"metrics port overflow", map[string]string{"METRICS_PORT": "70000"}},
		{"metrics port equals listen port", map[string]string{"LISTEN_PORT": "3000", "METRICS_PORT": "3000"}},
		{"bad action", map[string]string{"ACTION": "block"}},
		{"action out of range", map[string]string{"ACTION": "99"}},
		{"bad log level", map[string]string{"LOG_LEVEL": "verbose"}},
		{"negative mentions", map[string]string{"MAX_MENTIONS": "-1"}},
		{"bad mentions", map[string]string{"MAX_MENTIONS": "many"}},
		{"ratio NaN", map[string]string{"MAX_CONTENT_RATIO": "NaN"}},
		{"ratio over one", map[string]string{"MAX_CONTENT_RATIO": "1.5"}},
		{"bad body bytes", map[string]string{"MAX_BODY_BYTES": "huge"}},
		{"zero body bytes", map[string]string{"MAX_BODY_BYTES": "0"}},
		{"bad target", map[string]string{"MENTION_FILTER_TARGET": "bogus"}},
		{"bad timeout", map[string]string{"READ_TIMEOUT": "soon"}},
		{"negative timeout", map[string]string{"WRITE_TIMEOUT": "-5s"}},
		{"zero timeout", map[string]string{"SHUTDOWN_TIMEOUT": "0s"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if _, err := loadConfig(); err == nil {
				t.Errorf("expected error for %v, got nil", c.env)
			}
		})
	}
}

func TestLoadConfig_LocalDomain(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", ""},
		{"local.example.com", "local.example.com"},
		{"https://local.example.com/", "local.example.com"},
		{"LOCAL.EXAMPLE.COM", "local.example.com"},
		{"local.example.com:8443", "local.example.com"},
		{"  ", ""},
		{"local.example.com.", "local.example.com"},
		{"https://local.example.com./", "local.example.com"},
		{"[::1]", "::1"},
		{"[::1]:8080", "::1"},
		{"::1", "::1"},
	}

	for _, c := range cases {
		t.Setenv("LOCAL_DOMAIN", c.env)
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("LOCAL_DOMAIN=%q returned error: %v", c.env, err)
		}
		if cfg.localDomain != c.want {
			t.Errorf("LOCAL_DOMAIN=%q → localDomain=%q, want %q", c.env, cfg.localDomain, c.want)
		}
	}
}
