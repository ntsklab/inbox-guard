package filters_test

import (
	"testing"

	"github.com/ntsklab/inbox-guard/filters"
)

func TestReason(t *testing.T) {
	if got, want := filters.Reason("mentions", "count", 5, "max", 4), "mentions count=5 max=4"; got != want {
		t.Errorf("Reason() = %q, want %q", got, want)
	}
	// Odd trailing arg must not panic; it renders with an empty value.
	if got, want := filters.Reason("x", "key"), "x key="; got != want {
		t.Errorf("Reason() with odd args = %q, want %q", got, want)
	}
	if got, want := filters.Reason("x"), "x"; got != want {
		t.Errorf("Reason() without args = %q, want %q", got, want)
	}
}
