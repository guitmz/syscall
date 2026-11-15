package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// asmSyntax holds the comment marker and the syscall instruction per arch.
var asmSyntax = map[string]struct{ comment, call string }{
	"x64":     {";", "mov rax, %d\nsyscall\n"},
	"x32":     {";", "mov eax, %d\nsyscall\n"},
	"x86":     {";", "mov eax, %d\nint 0x80\n"},
	"ia32":    {";", "mov eax, %d\nint 0x80\n"},
	"arm64":   {"//", "mov x8, #%d\nsvc #0\n"},
	"riscv64": {"#", "li a7, %d\necall\n"},
}

// assemblySnippet builds a fragment, the caller still has to load real arguments.
func assemblySnippet(s syscall) (string, error) {
	syntax, ok := asmSyntax[s.Arch]
	if !ok {
		return "", fmt.Errorf("assembly examples support x64, x86, ia32, x32, arm64, and riscv64, not %s", s.Arch)
	}
	c, err := conventionFor(s.Arch)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	line := func(text string) { fmt.Fprintf(&b, "%s %s\n", syntax.comment, text) }
	line(fmt.Sprintf("%s on %s: %s", s.Name, s.Arch, signature(s)))
	line("Instruction fragment, set valid arguments before running it.")
	if s.Arch == "ia32" {
		line("Needs an x86_64 kernel with CONFIG_IA32_EMULATION.")
	}
	registers := []string{c.Arg0, c.Arg1, c.Arg2, c.Arg3, c.Arg4, c.Arg5}
	if s.Nargs == nil {
		line("Signature unknown, check the syscall docs for arguments.")
	} else {
		for i := 0; i < min(max(*s.Nargs, 0), len(registers)); i++ {
			arg := "unknown argument"
			if a := s.args()[i]; a != nil && *a != "" {
				arg = *a
			}
			line(fmt.Sprintf("%s: %s", registers[i], arg))
		}
	}
	if s.Kconfig != nil && *s.Kconfig != "" {
		line(*s.Kconfig + " may be needed, otherwise you get ENOSYS.")
	}
	for _, note := range c.Notes {
		line(note)
	}
	fmt.Fprintf(&b, syntax.call, s.Nr)
	line(c.Return + " holds the return value (negative errno on failure).")
	return b.String(), nil
}

func highlightAssembly(code string, color bool, theme string) string {
	if !color || theme == "mono" {
		return code
	}
	comment, opcode, operand := "245", "81", "222"
	if theme == "light" {
		comment, opcode, operand = "240", "25", "130"
	}
	commentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(comment))
	opcodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(opcode)).Bold(true)
	operandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(operand))
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			lines[i] = commentStyle.Render(line)
		} else if line != "" {
			instruction, operands, found := strings.Cut(line, " ")
			lines[i] = opcodeStyle.Render(instruction)
			if found {
				lines[i] += " " + operandStyle.Render(operands)
			}
		}
	}
	return strings.Join(lines, "\n")
}
