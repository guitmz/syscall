package main

import (
	"strings"
	"unicode"
)

// textEditor is a single line editor with Emacs style keys and a kill buffer.
type textEditor struct {
	value  string
	cursor int
	killed string
}

const maxEditorRunes = 128

// insert adds text at the cursor, flattening newlines and tabs to spaces,
// dropping other control characters and capping the length.
func (m *textEditor) insert(text string) {
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	r := []rune(m.value)
	p := min(max(0, m.cursor), len(r))
	add := []rune(text)
	add = add[:min(len(add), max(0, maxEditorRunes-len(r)))]
	if len(add) == 0 {
		return
	}
	m.value = string(r[:p]) + string(add) + string(r[p:])
	m.cursor = p + len(add)
}

func wordStart(r []rune, p int) int {
	for p > 0 && unicode.IsSpace(r[p-1]) {
		p--
	}
	for p > 0 && !unicode.IsSpace(r[p-1]) {
		p--
	}
	return p
}

func (m *textEditor) edit(key, text string) {
	r := []rune(m.value)
	p := min(max(0, m.cursor), len(r))
	m.cursor = p
	switch key {
	case "ctrl+a", "home":
		p = 0
	case "ctrl+e", "end":
		p = len(r)
	case "ctrl+b", "left":
		p = max(0, p-1)
	case "ctrl+f", "right":
		p = min(len(r), p+1)
	case "alt+b":
		p = wordStart(r, p)
	case "alt+f":
		for p < len(r) && !unicode.IsSpace(r[p]) {
			p++
		}
		for p < len(r) && unicode.IsSpace(r[p]) {
			p++
		}
	case "backspace", "ctrl+h":
		if p > 0 {
			r = append(r[:p-1], r[p:]...)
			p--
		}
	case "delete", "ctrl+d":
		if p < len(r) {
			r = append(r[:p], r[p+1:]...)
		}
	case "ctrl+k":
		m.killed = string(r[p:])
		r = r[:p]
	case "ctrl+u":
		m.killed = string(r[:p])
		r, p = r[p:], 0
	case "ctrl+w", "alt+backspace":
		start := wordStart(r, p)
		m.killed = string(r[start:p])
		r, p = append(r[:start], r[p:]...), start
	case "ctrl+y":
		m.insert(m.killed)
		return
	default:
		if key == "space" {
			text = " "
		}
		if text == " " || !strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") {
			m.insert(text)
		}
		return
	}
	m.value = string(r)
	m.cursor = p
}
