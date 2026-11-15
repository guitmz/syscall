package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type modalRow struct {
	text  string
	style lipgloss.Style
}

// overlay draws a bordered card of the given width centered over background.
func overlay(background string, termWidth, termHeight, width int, rows []modalRow) string {
	edge := strings.Repeat("─", width-2)
	lines := []string{tuiDim.Render("╭" + edge + "╮")}
	for _, r := range rows {
		lines = append(lines, tuiDim.Render("│")+" "+r.style.Render(fit(r.text, width-4))+" "+tuiDim.Render("│"))
	}
	lines = append(lines, tuiDim.Render("╰"+edge+"╯"))
	card := lipgloss.NewLayer(strings.Join(lines, "\n")).X((termWidth - width) / 2).Y((termHeight - len(lines)) / 2).Z(1)
	return lipgloss.NewCompositor(lipgloss.NewLayer(background), card).Render()
}

// keyBinding is one explorer key. section and keys group it in the help
// modal. label and key feed the command palette when key is set.
type keyBinding struct {
	section string
	keys    string
	help    string
	label   string
	key     string
}

var keyBindings = []keyBinding{
	{"NAVIGATION", "↑/↓  j/k  C-p/C-n", "Move the selection", "", ""},
	{"NAVIGATION", "PgUp/PgDn  C-v/M-v", "Scroll details", "", ""},
	{"NAVIGATION", "Enter / Esc", "Open or close details", "", ""},
	{"NAVIGATION", "q", "Quit", "Quit", "q"},
	{"EXPLORE", "/", "Search", "Search syscalls", "/"},
	{"EXPLORE", "A", "Set architecture", "Filter architecture", "A"},
	{"EXPLORE", "K", "Set kernel release", "Filter kernel", "K"},
	{"EXPLORE", "[ / ]", "Previous or next kernel", "", ""},
	{"EXPLORE", "c", "Compare arches", "Compare architectures", "c"},
	{"EXPLORE", "t", "Kernel timeline", "Show kernel timeline", "t"},
	{"EXPLORE", "D", "Diff two kernel releases", "Compare kernel releases", "D"},
	{"EXPLORE", "r", "Related syscalls", "Show related syscalls", "r"},
	{"EXPLORE", "1 to 9", "Jump to a related syscall", "", ""},
	{"EXPLORE", "a", "Assembly snippet", "Show assembly snippet", "a"},
	{"EXPLORE", "Y", "Copy the assembly snippet", "Copy assembly snippet", "Y"},
	{"EXPLORE", "g", "Jump to first scanned release", "Jump to introduction", "g"},
	{"EXPLORE", "G", "Jump to removal release", "Jump to removal", "G"},
	{"EXPLORE", "i", "Toggle the wide summary panel", "Toggle summary panel", "i"},
	{"EXPLORE", "*", "Toggle favorite", "Toggle favorite", "*"},
	{"EXPLORE", "f", "Show favorites only", "Show favorites", "f"},
	{"EXPLORE", "m", "Open the online man page", "Open man page", "m"},
	{"EXPLORE", "M", "Read the local man page", "Read local man page", "M"},
	{"EXPLORE", "s", "Open kernel source", "Open source", "s"},
	{"EXPLORE", "y", "Copy the signature", "Copy signature", "y"},
	{"EXPLORE", "C", "Copy a command that reopens this view", "Copy explorer command", "C"},
	{"EXPLORE", ":", "Command palette", "", ""},
	{"EXPLORE", "?", "This help", "Show keyboard help", "?"},
	{"SEARCH", "name:term  desc:term", "Search one field only", "", ""},
	{"SEARCH", "family:name", "Match a syscall family", "", ""},
	{"SEARCH", "arg:type  nargs:N", "Match argument type or count", "", ""},
	{"SEARCH", "fd:true", "Takes a file descriptor", "", ""},
	{"SEARCH", "C-a/e  C-b/f", "Start or end, back or forward", "", ""},
	{"SEARCH", "M-b/f", "Move by word", "", ""},
	{"SEARCH", "C-w/u/k", "Kill word, before or after", "", ""},
	{"SEARCH", "C-y", "Yank the last kill", "", ""},
	{"SEARCH", "Tab", "Complete with the first result", "", ""},
	{"SEARCH", "Enter / Esc / C-g", "Done searching", "", ""},
}

// helpModal overlays a scrollable key reference on the explorer.
func helpModal(background string, width, height, offset int) string {
	cardWidth := min(68, width-4)
	var lines []modalRow
	section := ""
	for _, b := range keyBindings {
		if b.section != section {
			if section != "" {
				lines = append(lines, modalRow{"", tuiText})
			}
			lines = append(lines, modalRow{b.section, tuiSection})
			section = b.section
		}
		for _, line := range wrapText(fmt.Sprintf("%-22s%s", b.keys, b.help), cardWidth-4) {
			lines = append(lines, modalRow{line, tuiText})
		}
	}
	visible := max(1, min(height-2, len(lines)+4)-4)
	offset = min(max(0, offset), max(0, len(lines)-visible))
	hint := "↑↓ scroll  ·  any other key closes"
	if len(lines) > visible {
		hint = fmt.Sprintf("↑↓ scroll  %d to %d of %d  ·  esc closes", offset+1, min(offset+visible, len(lines)), len(lines))
	}
	rows := append([]modalRow{{"EXPLORER KEYS", tuiAccent}}, lines[offset:min(len(lines), offset+visible)]...)
	for len(rows) < visible+1 {
		rows = append(rows, modalRow{"", tuiText})
	}
	return overlay(background, width, height, cardWidth, append(rows, modalRow{hint, tuiMuted}))
}
