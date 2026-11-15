package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

func snippet(name, arch, language, kernel string) (string, error) {
	rows, err := getSyscalls(name, arch, kernel)
	if err != nil {
		return "", err
	}
	if len(rows) != 1 {
		return "", errors.New("that matches more than one syscall, pick a name or number that matches just one")
	}
	s := rows[0]
	header := fmt.Sprintf("%s on %s: %s", s.Name, arch, signature(s))
	if arch == "ia32" {
		header += " (requires an x86-64 kernel with CONFIG_IA32_EMULATION)"
	}
	if k := deref(s.Kconfig); k != "" {
		header += " (requires " + k + " to avoid ENOSYS)"
	}
	switch language {
	case "c":
		return fmt.Sprintf("// %s\n// Compile for Linux on the specified architecture only.\n// Replace all zero arguments with valid values before running.\n#if !defined(__linux__)\n#error This example requires Linux\n#endif\n#define _GNU_SOURCE\n#include <unistd.h>\n\nint main(void) {\n    long result = syscall(%d, 0L, 0L, 0L, 0L, 0L, 0L);\n    return result == -1;\n}\n", header, s.Nr), nil
	case "go":
		architectures := map[string]string{"x64": "amd64", "x86": "386", "arm": "arm", "arm64": "arm64", "riscv64": "riscv64", "loongarch64": "loong64", "ppc64": "ppc64", "s390x": "s390x", "mips64": "mips64", "mipso32": "mips", "ia32": "386"}
		goarch, ok := architectures[arch]
		if !ok {
			return "", fmt.Errorf("go examples do not support architecture %q", arch)
		}
		return fmt.Sprintf("//go:build linux && %s\n\n// %s\n// Replace zero arguments with valid values before running.\npackage main\n\nimport (\n    \"fmt\"\n    \"syscall\"\n)\n\nfunc main() {\n    r1, _, errno := syscall.RawSyscall6(%d, 0, 0, 0, 0, 0, 0)\n    fmt.Println(r1, errno)\n}\n", goarch, header, s.Nr), nil
	case "asm":
		return assemblySnippet(s)
	default:
		return "", errors.New("language must be c, go, or asm")
	}
}

func sameConvention(a, b convention) bool {
	return a.Instruction == b.Instruction && a.Nr == b.Nr && a.Return == b.Return &&
		slices.Equal(a.registers(), b.registers()) && slices.Equal(a.Notes, b.Notes)
}

// compareLines lists s on every architecture, marking with ≠ whatever differs
// from the selected one.
func compareLines(s syscall, kernel string, width int) []string {
	rows, err := getSyscalls(s.Name, "", kernel)
	if err != nil {
		return []string{err.Error()}
	}
	baseline, baselineErr := conventionFor(s.Arch)
	lines := []string{tuiSection.Render("ARCHITECTURE COMPARISON"), fmt.Sprintf("%s · %d architectures", s.Name, len(rows)), "≠ differs from " + s.Arch, ""}
	muted := func(text string) {
		for _, part := range wrapText(text, width) {
			lines = append(lines, tuiMuted.Render(part))
		}
	}
	mark := func(differs bool) string {
		if differs {
			return "≠"
		}
		return " "
	}
	for _, r := range rows {
		differs := r.Nr != s.Nr || r.ABI != s.ABI || signature(r) != signature(s) || deref(r.Kconfig) != deref(s.Kconfig) || deref(r.Family) != deref(s.Family)
		lines = append(lines, wrapText(fmt.Sprintf("%s %s  #%d  %s", mark(differs), r.Arch, r.Nr, r.ABI), width)...)
		muted("  " + signature(r))
		if f := deref(r.Family); f != "" {
			muted("  Family: " + f)
		}
		if k := deref(r.Kconfig); k != "" {
			muted("  Kconfig: " + k)
		}
		if c, e := conventionFor(r.Arch); e == nil {
			muted(fmt.Sprintf("%s %s · nr %s · ret %s · args %s", mark(baselineErr == nil && !sameConvention(c, baseline)), c.Instruction, c.Nr, c.Return, strings.Join(c.registers(), ",")))
			for _, note := range c.Notes {
				muted("  • " + note)
			}
		}
	}
	return lines
}

func timelineLines(s syscall, width int) []string {
	d, err := loadDataset()
	if err != nil {
		return []string{err.Error()}
	}
	lines := []string{tuiSection.Render("KERNEL TIMELINE"), s.Name + " · " + s.Arch, "", "First seen   " + s.Since}
	if s.Removed != nil {
		lines = append(lines, "Removed      "+*s.Removed)
	} else {
		lines = append(lines, "Status       still in the latest syscall table")
	}
	if k := deref(s.Kconfig); k != "" {
		lines = append(lines, "Kconfig      "+k+" (may be disabled)")
	}
	lines = append(lines, "", tuiMuted.Render("Scanned syscall tables · ● present  ○ absent"))
	for _, v := range d.kernels[s.Arch] {
		mark := "○"
		if s.in(v) {
			mark = "●"
		}
		lines = append(lines, mark+"  "+v.String())
	}
	return wrapAll(lines, width)
}
