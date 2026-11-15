package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// paletteMatches lists the palette entries whose label contains the query.
func (m explorer) paletteMatches() []keyBinding {
	var out []keyBinding
	q := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	for _, b := range keyBindings {
		if b.key != "" && strings.Contains(strings.ToLower(b.label), q) {
			out = append(out, b)
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
			b := matches[min(m.paletteIndex, len(matches)-1)]
			m.palette = false
			return m.dispatchKey(b.key)
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
