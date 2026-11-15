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

var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"NAVIGATION", [][2]string{
		{"↑/↓  j/k  C-p/C-n", "Move the selection"},
		{"PgUp/PgDn  C-v/M-v", "Scroll details"},
		{"Enter / Esc", "Open or close details"},
		{"q", "Quit"},
	}},
	{"EXPLORE", [][2]string{
		{"/", "Search"},
		{"A / K", "Set architecture or kernel"},
		{"[ / ]", "Previous or next kernel"},
		{"c / t", "Compare arches or kernel timeline"},
		{"a / Y", "Assembly snippet, or copy it"},
		{"D", "Diff two kernel releases"},
		{"r / 1 to 9", "Related syscalls, or jump to one"},
		{"i", "Toggle the wide summary panel"},
		{"* / f", "Toggle favorite or favorites only"},
		{"m / M", "Online or local man page"},
		{"s", "Open kernel source"},
		{"y / C", "Copy signature or explorer command"},
		{":", "Command palette"},
		{"g / G", "Jump to first or removal release"},
	}},
	{"SEARCH", [][2]string{
		{"name:term  desc:term", "Search just one field"},
		{"family:name", "Match a syscall family"},
		{"arg:type  nargs:N", "Match argument type or count"},
		{"fd:true", "Takes a file descriptor"},
		{"C-a/e  C-b/f", "Start or end, back or forward"},
		{"M-b/f", "Move by word"},
		{"C-w/u/k", "Kill word, before or after"},
		{"C-y", "Yank the last kill"},
		{"Tab", "Complete with the first result"},
		{"Enter / Esc / C-g", "Done searching"},
	}},
}

// helpModal overlays a scrollable key reference on the explorer.
func helpModal(background string, width, height, offset int) string {
	cardWidth := min(68, width-4)
	var lines []modalRow
	for i, section := range helpSections {
		if i > 0 {
			lines = append(lines, modalRow{"", tuiText})
		}
		lines = append(lines, modalRow{section.title, tuiSection})
		for _, k := range section.keys {
			for _, line := range wrapText(fmt.Sprintf("%-22s%s", k[0], k[1]), cardWidth-4) {
				lines = append(lines, modalRow{line, tuiText})
			}
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
