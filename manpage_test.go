package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalManPageAndStaleResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "man"), []byte("#!/bin/sh\nprintf 'READ\\nA\\bA sample\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	result := loadManPage("x64:0:read", "read")().(manPageResult)
	if !strings.Contains(result.text, "A sample") || strings.Contains(result.text, "\b") {
		t.Fatalf("man output: %q", result.text)
	}
	m := explorer{manKey: "other", manText: "current", manLoading: true}
	next, _ := m.Update(result)
	m = next.(explorer)
	if m.manText != "current" || !m.manLoading {
		t.Fatal("stale man page replaced current request")
	}
}

func TestMissingLocalManPage(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result := loadManPage("x64:0:read", "read")().(manPageResult)
	if !strings.Contains(result.text, "unavailable") {
		t.Fatal(result.text)
	}
}

func TestLoadManPageRejectsEmptyName(t *testing.T) {
	result := loadManPage("k", "")().(manPageResult)
	if !strings.Contains(result.text, "Invalid") {
		t.Fatal(result.text)
	}
}
