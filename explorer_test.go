package main

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestExplorerSearchAndNavigation(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.matches) != 2 {
		t.Fatalf("got %d", len(m.matches))
	}
	m.input = "old"
	m.filter()
	if len(m.matches) != 1 || m.all[m.matches[0]].Name != "old" {
		t.Fatal("search did not find old syscall")
	}
	view := m.View().Content
	if !strings.Contains(view, "sys_old") || !strings.Contains(view, "int fd") {
		t.Fatalf("missing syscall details: %s", view)
	}
	m.input = "does-not-exist"
	m.filter()
	if !strings.Contains(m.View().Content, "No matching syscalls") {
		t.Fatal("missing empty state")
	}
	m.input = ""
	m.filter()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	next := updated.(explorer)
	if next.cursor != 1 {
		t.Fatal("down did not select next result")
	}
	next.width = 39
	if !strings.Contains(next.View().Content, "Resize terminal") {
		t.Fatal("missing small terminal guidance")
	}
}
func TestFilterFieldKillAndYank(t *testing.T) {
	m := explorer{fieldMode: "architecture", fieldInput: "arm64", fieldCursor: 5}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = next.(explorer)
	if m.fieldInput != "" {
		t.Fatalf("kill left %q", m.fieldInput)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	m = next.(explorer)
	if m.fieldInput != "arm64" {
		t.Fatalf("yank restored %q", m.fieldInput)
	}
}

func TestExplorerLayouts(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ w, h int }{{40, 12}, {60, 18}, {80, 24}, {120, 30}, {140, 24}, {180, 30}} {
		m.width, m.height = size.w, size.h
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size.h {
			t.Errorf("%dx%d: %d lines", size.w, size.h, len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > size.w {
				t.Errorf("%dx%d: line width %d: %q", size.w, size.h, lipgloss.Width(line), line)
			}
		}
	}
	m.width, m.height = 60, 18
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(explorer)
	if !m.showDetails || !strings.Contains(m.View().Content, "sys_old") {
		t.Fatal("compact details not shown")
	}
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = model.(explorer)
	if m.showDetails {
		t.Fatal("escape did not return to results")
	}
}

func TestWideSummaryAndAssemblyRegisters(t *testing.T) {
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 160, 30
	view := ansi.Strip(m.View().Content)
	for _, text := range []string{"ARCHITECTURES", "KERNEL TIMELINE", "RELATED"} {
		if !strings.Contains(view, text) {
			t.Fatalf("wide summary missing %q", text)
		}
	}
	m.viewMode = "asm"
	if !strings.Contains(ansi.Strip(m.View().Content), "REGISTERS") {
		t.Fatal("assembly register panel missing")
	}
	m.hideSummary = true
	if strings.Contains(ansi.Strip(m.View().Content), "REGISTERS") {
		t.Fatal("hidden panel is still shown")
	}
}

func TestExplorerColumnHeaders(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{60, 100} {
		m.width = width
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		var header, row string
		for _, line := range lines {
			if strings.Contains(line, "NAME") && strings.Contains(line, "ARCH") {
				header = line
			}
			if strings.Contains(line, "old") && strings.Contains(line, "x64") {
				row = line
			}
		}
		if header == "" || row == "" {
			t.Fatalf("width %d: missing header or row", width)
		}
		if lipgloss.Width(header[:strings.Index(header, "NAME")]) != lipgloss.Width(row[:strings.Index(row, "old")]) {
			t.Fatalf("width %d: names are misaligned", width)
		}
		if lipgloss.Width(header[:strings.Index(header, "ARCH")])+len("ARCH") != lipgloss.Width(row[:strings.Index(row, "x64")])+len("x64") {
			t.Fatalf("width %d: architectures are misaligned", width)
		}
	}
}

func TestSearchEmacsEditing(t *testing.T) {
	m := explorer{input: "hello world", inputCursor: 11}
	m.editSearch("ctrl+a", "")
	if m.inputCursor != 0 {
		t.Fatal("ctrl+a")
	}
	m.editSearch("ctrl+f", "")
	m.editSearch("ctrl+d", "")
	if m.input != "hllo world" {
		t.Fatalf("delete: %q", m.input)
	}
	m.editSearch("ctrl+e", "")
	m.editSearch("ctrl+w", "")
	if m.input != "hllo " || m.killed != "world" {
		t.Fatalf("kill word: %q %q", m.input, m.killed)
	}
	m.editSearch("ctrl+y", "")
	if m.input != "hllo world" {
		t.Fatalf("yank: %q", m.input)
	}
	m.editSearch("alt+b", "")
	m.editSearch("ctrl+k", "")
	if m.input != "hllo " || m.killed != "world" {
		t.Fatal("kill to end")
	}
	m.editSearch("ctrl+u", "")
	if m.input != "" || m.killed != "hllo " {
		t.Fatal("kill to start")
	}
	m.editSearch("ctrl+y", "")
	if m.input != "hllo " {
		t.Fatalf("yank lost space: %q", m.input)
	}
	m.input = "αβ"
	m.inputCursor = 2
	m.editSearch("ctrl+b", "")
	m.editSearch("backspace", "")
	if m.input != "β" || m.inputCursor != 0 {
		t.Fatalf("unicode edit: %q %d", m.input, m.inputCursor)
	}
	m.editSearch("", "猫\nread")
	if m.input != "猫 readβ" {
		t.Fatalf("paste: %q", m.input)
	}
}
func TestEmacsNavigationAndCursor(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	m = updated.(explorer)
	if m.cursor != 1 {
		t.Fatal("ctrl+n did not move")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	m = updated.(explorer)
	if m.cursor != 0 {
		t.Fatal("ctrl+p did not move")
	}
	m.input = "old"
	m.inputCursor = 1
	m.searching = true
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "SEARCH  old") {
		t.Fatalf("cursor changed query text: %s", view)
	}
}

func TestHelpModalOverlayAndScrolling(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []struct{ w, h int }{{40, 10}, {60, 18}, {100, 30}} {
		m.width, m.height = size.w, size.h
		m.showHelp = true
		m.helpOffset = 0
		view := ansi.Strip(m.View().Content)
		lines := strings.Split(view, "\n")
		if !strings.Contains(view, "EXPLORER KEYS") || !strings.Contains(view, "SYSCALL") {
			t.Fatalf("missing modal or underlying explorer at %dx%d: %s", size.w, size.h, view)
		}
		if len(lines) > size.h {
			t.Fatalf("modal exceeds terminal height: %d", len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > size.w {
				t.Fatalf("modal exceeds terminal width: %q", line)
			}
		}
		top := -1
		for i, line := range lines {
			if strings.Contains(line, "╭") {
				top = i
				break
			}
		}
		if top < 0 || top > (size.h/2) {
			t.Fatalf("modal not centered: top %d", top)
		}
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(explorer)
		if m.helpOffset != 1 || !m.showHelp {
			t.Fatal("help scroll failed")
		}
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		m = next.(explorer)
		if m.showHelp || m.helpOffset != 0 {
			t.Fatal("help close failed")
		}
	}
}

func TestHelpModalUsesTerminalBackgroundAndMasksContent(t *testing.T) {
	background := strings.TrimSuffix(strings.Repeat(strings.Repeat("#", 100)+"\n", 30), "\n")
	output := helpModal(background, 100, 30, 0)
	if strings.Contains(output, "\x1b[48;5;235m") {
		t.Fatal("modal forces a background color")
	}
	lines := strings.Split(ansi.Strip(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "│") {
			left := strings.Index(line, "│")
			right := strings.LastIndex(line, "│")
			if right > left && strings.Contains(line[left:right], "#") {
				t.Fatalf("background leaked through modal: %q", line)
			}
		}
	}
}

func TestExplorerReleaseDiff(t *testing.T) {
	useFixture(t)
	m, err := newExplorer("x64", "all")
	if err != nil {
		t.Fatal(err)
	}
	m.fieldMode = "diff"
	m.fieldInput = "6.1 6.2"
	m.fieldCursor = len(m.fieldInput)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(explorer)
	if m.viewMode != "diff" || !strings.Contains(ansi.Strip(strings.Join(m.currentDetail(60), "\n")), "+ new") {
		t.Fatal("diff view failed")
	}
	if !strings.Contains(m.shareCommand(), "--diff-from '6.1'") {
		t.Fatal("diff view is not shareable")
	}
}
func TestExplorerRelatedAndCompletion(t *testing.T) {
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "name:read"
	m.filter()
	m.viewMode = "related"
	if len(m.relatedIndices()) == 0 {
		t.Fatal("related syscalls missing")
	}
	m.jumpRelated(1)
	if m.input != "name:read" || !strings.Contains(m.status, "hidden") {
		t.Fatalf("jump wiped the query: %q %q", m.input, m.status)
	}
	m.input = ""
	m.filter()
	m.selectKey("x64:0:read")
	m.jumpRelated(1)
	if m.selected() == nil || m.selected().Name != "write" || m.input != "" {
		t.Fatalf("related jump: %+v", m.selected())
	}
	m.searching = true
	m.input = "rea"
	m.filter()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(explorer)
	if m.input != "read" {
		t.Fatalf("completion: %q", m.input)
	}
}

func TestThemeAndSearchSuggestions(t *testing.T) {
	m, err := newExplorer("amd64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.searching = true
	m.input = "rea"
	m.filter()
	t.Cleanup(func() {
		if err := applyTheme("dark"); err != nil {
			t.Error(err)
		}
	})
	for _, theme := range []string{"dark", "light", "mono"} {
		if err := applyTheme(theme); err != nil {
			t.Fatal(err)
		}
		for _, size := range []struct{ w, h int }{{40, 10}, {80, 24}, {120, 30}} {
			m.width, m.height = size.w, size.h
			view := m.View().Content
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.w {
					t.Fatalf("%s %dx%d: overflow", theme, size.w, size.h)
				}
			}
			if theme == "mono" && strings.Contains(view, "\x1b[38;") {
				t.Fatal("mono rendered foreground color")
			}
		}
	}
	m.width = 120
	if !strings.Contains(ansi.Strip(m.View().Content), "tab → read") {
		t.Fatal("missing completion hint")
	}
}

func TestOversizedInitialSearchCanBeEdited(t *testing.T) {
	m := explorer{input: strings.Repeat("a", 140), inputCursor: 140}
	m.editSearch("", "paste")
	if len(m.input) != 140 {
		t.Fatal("oversized input changed on paste")
	}
	m.editSearch("x", "x")
	if len(m.input) != 140 {
		t.Fatal("oversized input changed on insertion")
	}
	m.editSearch("backspace", "")
	if len(m.input) != 139 {
		t.Fatal("oversized input cannot be edited")
	}
}

func TestExplorerSearchRestoresCompactDetails(t *testing.T) {
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.showDetails = 60, true
	for _, k := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: tea.KeyEscape}} {
		next, _ := m.Update(k)
		m = next.(explorer)
		if k.Code == '/' && m.showDetails {
			t.Fatal("search should hide compact details")
		}
	}
	if !m.showDetails {
		t.Fatal("leaving search should restore details")
	}
	if m.archNumbers["read"]["arm64"] != 63 {
		t.Fatalf("arch index: %v", m.archNumbers["read"])
	}
}

func TestOpenReferenceRejectsUnsafeLinks(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://u:p@example.com", "https://:8080/", "file:///etc/passwd", ""} {
		if err := openReference(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestExplorerDiffFlagsNeedDiffView(t *testing.T) {
	err := runExplorer(io.Discard, explorerStart{view: "asm", theme: "auto", diffFrom: "6.1"})
	if err == nil || !strings.Contains(err.Error(), "diff view") {
		t.Fatalf("got %v", err)
	}
}
