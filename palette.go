package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type paletteCommand struct{ label, key string }

var paletteCommands = []paletteCommand{
	{"Search syscalls", "/"},
	{"Filter architecture", "A"},
	{"Filter kernel", "K"},
	{"Compare architectures", "c"},
	{"Show kernel timeline", "t"},
	{"Compare kernel releases", "D"},
	{"Show related syscalls", "r"},
	{"Show assembly snippet", "a"},
	{"Copy assembly snippet", "Y"},
	{"Jump to introduction", "g"},
	{"Jump to removal", "G"},
	{"Toggle favorite", "*"},
	{"Show favorites", "f"},
	{"Copy signature", "y"},
	{"Copy explorer command", "C"},
	{"Open man page", "m"},
	{"Read local man page", "M"},
	{"Toggle summary panel", "i"},
	{"Open source", "s"},
	{"Show keyboard help", "?"},
	{"Quit", "q"},
}

func (m explorer) paletteMatches() []paletteCommand {
	var out []paletteCommand
	q := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	for _, c := range paletteCommands {
		if strings.Contains(strings.ToLower(c.label), q) {
			out = append(out, c)
		}
	}
	return out
}

func (m *explorer) appendPaletteQuery(text string) {
	ed := textEditor{value: m.paletteQuery, cursor: len([]rune(m.paletteQuery))}
	ed.insert(text)
	if ed.value != m.paletteQuery {
		m.paletteQuery = ed.value
		m.paletteIndex = 0
	}
}

func (m explorer) paletteUpdate(key, text string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "ctrl+g":
		m.palette = false
	case "up", "ctrl+p":
		m.paletteIndex = max(0, m.paletteIndex-1)
	case "down", "ctrl+n":
		m.paletteIndex = min(max(0, len(m.paletteMatches())-1), m.paletteIndex+1)
	case "enter":
		if matches := m.paletteMatches(); len(matches) > 0 {
			key := matches[min(m.paletteIndex, len(matches)-1)].key
			m.palette = false
			return m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
		}
	case "backspace":
		if r := []rune(m.paletteQuery); len(r) > 0 {
			m.paletteQuery = string(r[:len(r)-1])
			m.paletteIndex = 0
		}
	default:
		if !strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") {
			m.appendPaletteQuery(text)
		}
	}
	return m, nil
}

func (m explorer) paletteModal(background string) string {
	width := min(58, m.width-4)
	visible := max(1, min(m.height-2, 14)-5)
	matches := m.paletteMatches()
	start := max(0, m.paletteIndex-visible+1)
	rows := []modalRow{{"COMMAND PALETTE", tuiAccent}, {"› " + m.paletteQuery, tuiText}}
	for idx := start; idx < start+visible; idx++ {
		row := modalRow{"", tuiText}
		if idx < len(matches) {
			row.text = matches[idx].label + "  [" + matches[idx].key + "]"
			if idx == m.paletteIndex {
				row.style = tuiSelected
			}
		}
		rows = append(rows, row)
	}
	return overlay(background, m.width, m.height, width, append(rows, modalRow{"↑↓ select · enter run · esc close", tuiMuted}))
}
