package main

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSearchCompareAndSnippets(t *testing.T) {
	useFixture(t)
	cases := []struct {
		args     []string
		contains string
	}{
		{[]string{"search", "ol", "--kernel", "all", "--output", "json"}, `"name": "old"`},
		{[]string{"search", "n", "--arch", "x64"}, "new"},
		{[]string{"get", "old", "--kernel", "all"}, "old(int fd)"},
		{[]string{"get", "old", "--kernel", "all", "--output", "json"}, `"removed_in": "6.2"`},
		{[]string{"snippet", "new", "--arch", "x64", "--language", "asm"}, "mov rax, 1"},
		{[]string{"snippet", "new", "--arch", "x64", "--language", "c"}, "syscall(1,"},
		{[]string{"version", "--output", "json"}, `"dataset_sha256":`},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := run(tc.args, &out); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.contains) {
			t.Errorf("%v: %s", tc.args, out.String())
		}
	}
	code, err := snippet("new", "x64", "go", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parser.ParseFile(token.NewFileSet(), "snippet.go", code, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
	if _, err = snippet("new", "mips64", "asm", ""); err == nil {
		t.Fatal("expected unsupported architecture")
	}
}
func TestExplorerViewsAndFilters(t *testing.T) {
	useFixture(t)
	if _, err := newExplorer("missing", ""); err == nil {
		t.Fatal("expected unknown architecture error")
	}
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "name:old"
	m.filter()
	if len(m.matches) != 1 {
		t.Fatal("name filter")
	}
	m.input = "desc:absent"
	m.filter()
	if len(m.matches) != 0 {
		t.Fatal("description filter")
	}
	m.input = ""
	m.filter()
	m.viewMode = "compare"
	if !strings.Contains(strings.Join(m.currentDetail(70), "\n"), "ARCHITECTURE COMPARISON") {
		t.Fatal("compare view")
	}
	m.viewMode = "timeline"
	if !strings.Contains(strings.Join(m.currentDetail(70), "\n"), "6.1") {
		t.Fatal("timeline")
	}
	if err = m.setFilters("x64", "6.2"); err != nil {
		t.Fatal(err)
	}
	if len(m.matches) != 1 || m.all[m.matches[0]].Name != "new" {
		t.Fatal("kernel filter")
	}
	if err = m.shiftKernel(-1); err != nil {
		t.Fatal(err)
	}
	if m.kernel != "6.1" {
		t.Fatal("kernel selection")
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if !updated.(explorer).showHelp {
		t.Fatal("help view")
	}
}
func TestFavorites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := map[string]bool{"x64:0:read": true, "x64:1:write": true}
	if err := saveFavorites(m); err != nil {
		t.Fatal(err)
	}
	got, err := readFavorites()
	if err != nil {
		t.Fatal(err)
	}
	if !got["x64:0:read"] || len(got) != 2 {
		t.Fatalf("%v", got)
	}
	p, err := favoritesPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || !json.Valid(b) {
		t.Fatalf("invalid favorites: %v", err)
	}
}

func TestFavoritesConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	errs := make(chan error, 2)
	for _, key := range []string{"read", "write"} {
		go func() { _, err := toggleFavorite(key); errs <- err }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	got, err := readFavorites()
	if err != nil || !got["read"] || !got["write"] {
		t.Fatalf("favorites: %v, error: %v", got, err)
	}
}
