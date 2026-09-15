package main

import (
	"os"
	"testing"
)

// TestMain points the demo's persistence at a throwaway config directory so
// tests are hermetic and never read or write the developer's real layout.
func TestMain(m *testing.M) {
	if dir, err := os.MkdirTemp("", "tidedeck-test"); err == nil {
		_ = os.Setenv("XDG_CONFIG_HOME", dir)
		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}
