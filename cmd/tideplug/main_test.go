package main

import (
	"bytes"
	"strings"
	"testing"
)

// A sound plugin exits 0, a broken one 1, and a wrong invocation 2, so a
// script or CI can act on the result.
func TestExitStatus(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"validate", "../../contrib/favorites"}, &out, &errs); code != 0 {
		t.Fatalf("bundled plugin exited %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "✓ manifest: Favorites") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	if code := run([]string{"validate", t.TempDir()}, &out, &errs); code != 1 || !strings.Contains(out.String(), "✗ manifest") {
		t.Fatalf("empty dir exited %d:\n%s", code, out.String())
	}
	if code := run(nil, &out, &errs); code != 2 || !strings.Contains(errs.String(), "usage") {
		t.Fatalf("no args exited %d", code)
	}
}
