package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestHexLookupAndSignatureSearch(t *testing.T) {
	useFixture(t)
	tests := []struct {
		args []string
		want string
	}{{[]string{"get", "0x0", "-a", "x64", "-k", "all"}, "old"}, {[]string{"search", "arg:fd", "-k", "all", "-o", "json"}, `"name": "old"`}, {[]string{"search", "nargs:0"}, "new"}, {[]string{"search", "fd:true", "-k", "all"}, "old"}}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := run(tt.args, &out); err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		if !strings.Contains(out.String(), tt.want) {
			t.Errorf("%v: %q", tt.args, out.String())
		}
	}
	var out bytes.Buffer
	if err := run([]string{"get", "0xno", "-a", "x64"}, &out); err == nil {
		t.Fatal("bad hex accepted")
	}
}

func TestTraceAndHistory(t *testing.T) {
	useFixture(t)
	var out bytes.Buffer
	input := "0x0 argument\n1 argument\nunrecognized\n"
	if err := annotateTrace(strings.NewReader(input), &out, "x64", "all", "table", traceOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "old(int fd)") || !strings.Contains(out.String(), "new()") || !strings.Contains(out.String(), "unrecognized\n") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := annotateTrace(strings.NewReader(input), &out, "x64", "all", "json", traceOptions{}); err != nil {
		t.Fatal(err)
	}
	var records []traceLine
	if err := json.Unmarshal(out.Bytes(), &records); err != nil || len(records) != 3 || len(records[0].Matches) != 1 {
		t.Fatalf("%s %v", out.String(), err)
	}
	h, err := syscallHistory("old", "x64", "6.1", "6.2")
	if err != nil {
		t.Fatal(err)
	}
	if c := h.Comparison; c == nil || !c.PresentFrom || c.PresentTo {
		t.Fatalf("%+v", h)
	}
	if _, err = syscallHistory("old", "arm64", "6.1", "6.2"); err == nil {
		t.Fatal("expected coverage error")
	}
}

func TestMarkdownCompareAndShare(t *testing.T) {
	useFixture(t)
	var out bytes.Buffer
	if err := run([]string{"old", "-k", "all"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "old(int fd)") || !strings.Contains(out.String(), "REMOVED") {
		t.Fatal(out.String())
	}
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "name:old's"
	m.theme = "light"
	m.viewMode = "compare"
	cmd := m.shareCommand()
	if !strings.Contains(cmd, `'name:old'\''s'`) || !strings.Contains(cmd, "--select") || !strings.Contains(cmd, "--view 'compare'") {
		t.Fatal(cmd)
	}
}

func TestExplorerPalettePaste(t *testing.T) {
	m := explorer{palette: true, paletteQuery: "show ", paletteIndex: 3}
	next, _ := m.Update(tea.PasteMsg{Content: "kernel\nrelease"})
	m = next.(explorer)
	if m.paletteQuery != "show kernel release" || m.paletteIndex != 0 {
		t.Fatalf("paste produced query %q, index %d", m.paletteQuery, m.paletteIndex)
	}
	next, _ = m.paletteUpdate("text", strings.Repeat("x", maxEditorRunes)+"\x1b")
	m = next.(explorer)
	if got := len([]rune(m.paletteQuery)); got != maxEditorRunes || strings.ContainsRune(m.paletteQuery, '\x1b') {
		t.Fatalf("typed query has %d runes: %q", got, m.paletteQuery)
	}
}

func TestExplorerPaletteMouseAndTheme(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	m.width = 100
	m.height = 24
	m.palette = true
	m.paletteQuery = "timeline"
	matches := m.paletteMatches()
	if len(matches) != 1 || matches[0].key != "t" {
		t.Fatal(matches)
	}
	v := m.View()
	if !strings.Contains(v.Content, "COMMAND PALETTE") || v.MouseMode != tea.MouseModeCellMotion {
		t.Fatal("palette or mouse missing")
	}
	next, _ := m.paletteUpdate("enter", "")
	m = next.(explorer)
	if m.viewMode != "timeline" {
		t.Fatal("palette action not applied")
	}
	m.viewMode = ""
	next, _ = m.Update(tea.MouseClickMsg{X: 3, Y: 6, Button: tea.MouseLeft})
	m = next.(explorer)
	if m.cursor != 1 {
		t.Fatal("mouse click selection")
	}
	next, _ = m.Update(tea.MouseWheelMsg{X: 2, Y: 8, Button: tea.MouseWheelUp})
	m = next.(explorer)
	if m.cursor != 0 {
		t.Fatal("wheel selection")
	}
	if err = applyTheme("mono"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tuiAccent.Render("x"), "38;") {
		t.Fatal("mono has color")
	}
	if err = applyTheme("dark"); err != nil {
		t.Fatal(err)
	}
	if err = applyTheme("invalid"); err == nil {
		t.Fatal("unknown theme accepted")
	}
}

func TestTwoArchitectureComparisonAndExplorerFilters(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]string{"arg:fd": "old", "nargs:0": "new", "0x0": "old"} {
		m.input = query
		m.filter()
		if len(m.matches) != 1 || m.all[m.matches[0]].Name != want {
			t.Fatalf("%s: %v", query, m.matches)
		}
	}
	m.width = 40
	m.height = 10
	m.palette = true
	m.paletteQuery = ""
	view := m.View().Content
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("palette overflow: %q", line)
		}
	}
}
