package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAssemblySnippets(t *testing.T) {
	for _, tc := range []struct{ arch, instruction, argument string }{
		{"x64", "mov rax, 0\nsyscall", "rdi: unsigned int fd"},
		{"arm64", "mov x8, #63\nsvc #0", "x0: unsigned int fd"},
		{"riscv64", "li a7, 63\necall", "a0: unsigned int fd"},
		{"x86", "mov eax, 3\nint 0x80", "ebx: unsigned int fd"},
		{"ia32", "mov eax, 3\nint 0x80", "ebx: unsigned int fd"},
		{"x32", "mov eax, 1073741824\nsyscall", "rdi: unsigned int fd"},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			code, err := snippet("read", tc.arch, "asm", "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(code, tc.instruction) || !strings.Contains(code, tc.argument) || !strings.Contains(code, "Instruction fragment") {
				t.Fatal(code)
			}
		})
	}
}

func TestAssemblyOutputAndExplorer(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"snippet", "read", "-a", "x64", "-l", "asm", "--color", "always", "-o", "json"}, &out); err != nil {
		t.Fatal(err)
	}
	var value struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(out.Bytes(), &value); err != nil || strings.Contains(value.Code, "\x1b") {
		t.Fatalf("JSON code is not plain text: %q (%v)", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"snippet", "read", "-a", "x64", "-l", "asm"}, &out); err != nil || strings.Contains(out.String(), "\x1b") {
		t.Fatalf("redirected output: %q (%v)", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"snippet", "read", "-a", "x64", "-l", "asm", "--color", "always"}, &out); err != nil || !strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("forced color output: %q (%v)", out.String(), err)
	}
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(explorer)
	if m.viewMode != "asm" || !strings.Contains(strings.Join(m.currentDetail(120), "\n"), "ASSEMBLY SNIPPET") {
		t.Fatal("assembly view did not open")
	}
}

func TestAssemblyThemes(t *testing.T) {
	code := "; comment\nmov rax, 0\n"
	dark := highlightAssembly(code, true, "dark")
	light := highlightAssembly(code, true, "light")
	if dark == light || !strings.Contains(dark, "\x1b[") || !strings.Contains(light, "\x1b[") {
		t.Fatalf("themes not distinguished: %q / %q", dark, light)
	}
	if got := highlightAssembly(code, true, "mono"); got != code {
		t.Fatalf("mono output contains styling: %q", got)
	}
	if got := highlightAssembly(code, false, "dark"); got != code {
		t.Fatalf("disabled color contains styling: %q", got)
	}
	var out bytes.Buffer
	if err := run([]string{"snippet", "read", "-a", "x64", "-l", "asm", "--theme", "unknown"}, &out); err == nil {
		t.Fatal("invalid theme accepted")
	}
}
