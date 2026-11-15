package main

import (
	"strings"
	"testing"
)

func TestTextEditorInsertion(t *testing.T) {
	ed := textEditor{value: "αβ", cursor: 1}
	ed.edit("text", "猫\n\x1b")
	if ed.value != "α猫 β" || ed.cursor != 3 {
		t.Fatalf("insertion: value %q, cursor %d", ed.value, ed.cursor)
	}
	ed.insert(strings.Repeat("x", maxEditorRunes))
	if got := len([]rune(ed.value)); got != maxEditorRunes {
		t.Fatalf("inserted %d runes, want %d", got, maxEditorRunes)
	}
}
