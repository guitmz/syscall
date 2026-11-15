package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

type display struct {
	color bool
	width int
}

func newDisplay(out io.Writer, mode string) display {
	// Pipes have no terminal width, so keep each record on one line and let
	// tools like fzf and less decide how to wrap it.
	d := display{width: 1 << 20}
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		d.width = 80
		if w, _, e := term.GetSize(int(f.Fd())); e == nil && w > 0 {
			d.width = w
		}
		d.color = os.Getenv("TERM") != "dumb" && os.Getenv("NO_COLOR") == ""
	}
	switch mode {
	case "always":
		d.color = true
	case "never":
		d.color = false
	}
	return d
}

func (d display) paint(s, code string) string {
	if !d.color || s == "" {
		return s
	}
	style := lipgloss.NewStyle()
	switch code {
	case "1;36":
		style = style.Bold(true).Foreground(lipgloss.Color("6"))
	case "32":
		style = style.Foreground(lipgloss.Color("2"))
	case "31":
		style = style.Foreground(lipgloss.Color("1"))
	case "2":
		style = style.Faint(true)
	case "1":
		style = style.Bold(true)
	}
	return style.Render(s)
}

func (d display) table(out io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i, v := range row {
			widths[i] = max(widths[i], lipgloss.Width(v))
		}
	}
	total := -2
	for _, n := range widths {
		total += n + 2
	}
	var b strings.Builder
	if total > d.width {
		// Too wide to fit, so stack each record and wrap its values.
		for i, row := range rows {
			if i > 0 {
				b.WriteByte('\n')
			}
			for j, v := range row {
				prefix := headers[j] + ": "
				indent := lipgloss.Width(prefix)
				for k, line := range wrapText(v, d.width-indent) {
					if k == 0 {
						b.WriteString(d.paint(prefix, "1;36"))
					} else {
						b.WriteString(strings.Repeat(" ", indent))
					}
					b.WriteString(line + "\n")
				}
			}
		}
		_, err := io.WriteString(out, b.String())
		return err
	}
	colorChange := len(headers) > 1 && headers[1] == "CHANGE"
	for i, row := range append([][]string{headers}, rows...) {
		for j, cell := range row {
			switch {
			case i == 0:
				cell = d.paint(cell, "1;36")
			case colorChange && j == 1 && cell == "added":
				cell = d.paint(cell, "32")
			case colorChange && j == 1 && cell == "removed":
				cell = d.paint(cell, "31")
			}
			b.WriteString(cell)
			if j < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[j]-lipgloss.Width(cell)+2))
			}
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func wrapText(s string, width int) []string {
	return strings.Split(lipgloss.Wrap(s, max(1, width), ""), "\n")
}

// syscalls prints one row per syscall, leaving out the arch column when
// every row has the same one and the family column when none has one.
func (d display) syscalls(out io.Writer, ss []syscall) error {
	showArch := slices.ContainsFunc(ss, func(s syscall) bool { return s.Arch != ss[0].Arch })
	showFamily := slices.ContainsFunc(ss, func(s syscall) bool { return deref(s.Family) != "" })
	pick := func(cells ...string) []string {
		row := []string{}
		if showArch {
			row = append(row, cells[0])
		}
		row = append(row, cells[1:4]...)
		if showFamily {
			row = append(row, cells[4])
		}
		return append(row, cells[5])
	}
	rows := [][]string{}
	for _, s := range ss {
		rows = append(rows, pick(s.Arch, strconv.Itoa(s.Nr), s.Name, s.Since, dash(s.Family), deref(s.Description)))
	}
	return d.table(out, pick("ARCH", "NR", "NAME", "SINCE", "FAMILY", "DESCRIPTION"), rows)
}

func (d display) architectures(out io.Writer, arches []architecture) error {
	rows := [][]string{}
	for _, a := range arches {
		rows = append(rows, []string{a.Arch, a.First, a.Last, orDash(a.Until), strconv.Itoa(a.Syscalls)})
	}
	return d.table(out, []string{"ARCH", "FIRST", "LAST", "REMOVED IN", "SYSCALLS"}, rows)
}

func (d display) diff(out io.Writer, diff kernelDiff) error {
	rows := [][]string{}
	for _, a := range diff.Arches {
		for _, s := range a.Added {
			rows = append(rows, []string{a.Arch, "added", strconv.Itoa(s.Nr), s.Name})
		}
		for _, s := range a.Removed {
			rows = append(rows, []string{a.Arch, "removed", strconv.Itoa(s.Nr), s.Name})
		}
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(out, "No syscall changes.")
		return err
	}
	return d.table(out, []string{"ARCH", "CHANGE", "NR", "NAME"}, rows)
}

func (d display) history(out io.Writer, h history) error {
	if c := h.Comparison; c != nil {
		_, err := fmt.Fprintf(out, "%s: %t → %s: %t\n", c.From, c.PresentFrom, c.To, c.PresentTo)
		return err
	}
	// Collapse runs of releases with the same status into one row.
	rows := [][]string{}
	for i, p := range h.Releases {
		status := "absent"
		if p.Present {
			status = "present"
		}
		if i > 0 && rows[len(rows)-1][2] == status {
			rows[len(rows)-1][1] = p.Version
			continue
		}
		rows = append(rows, []string{p.Version, p.Version, status})
	}
	for _, row := range rows {
		if row[0] != row[1] {
			row[0] += " to " + row[1]
		}
		row[1] = row[2]
	}
	if _, err := fmt.Fprintf(out, "%s on %s\n\n", d.paint(h.Syscall.Name, "1;36"), h.Syscall.Arch); err != nil {
		return err
	}
	table := [][]string{}
	for _, row := range rows {
		table = append(table, row[:2])
	}
	return d.table(out, []string{"KERNELS", "STATUS"}, table)
}

// signature renders s like a C prototype, or "unknown" without argument data.
func signature(s syscall) string {
	if s.Nargs == nil {
		return "unknown"
	}
	var args []string
	for _, a := range s.args() {
		if a != nil && *a != "" {
			args = append(args, *a)
		}
	}
	return s.Name + "(" + strings.Join(args, ", ") + ")"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// dash renders an optional field, or a dash when it is missing.
func dash(s *string) string { return orDash(deref(s)) }

// details prints one syscall as labeled, wrapped fields.
func (d display) details(out io.Writer, s syscall) error {
	pairs := [][2]string{{"NAME", s.Name}, {"ARCH", s.Arch}, {"NUMBER", fmt.Sprintf("%d (%#x)", s.Nr, s.Nr)}, {"ABI", s.ABI}}
	add := func(label, value string) {
		if value != "" {
			pairs = append(pairs, [2]string{label, value})
		}
	}
	if s.Nargs != nil {
		pairs = append(pairs, [2]string{"SIGNATURE", signature(s)})
	}
	if s.Description != nil {
		pairs = append(pairs, [2]string{"DESCRIPTION", *s.Description})
	}
	if c, err := conventionFor(s.Arch); err == nil {
		regs := c.registers()[:min(6, nargs(s))]
		call := c.Instruction + ", number in " + c.Nr
		if len(regs) > 0 {
			call += ", args in " + strings.Join(regs, " ")
		}
		pairs = append(pairs, [2]string{"CALL", call + ", result in " + c.Return})
	}
	pairs = append(pairs, [2]string{"SINCE", s.Since})
	if s.Removed != nil {
		pairs = append(pairs, [2]string{"REMOVED IN", *s.Removed})
	}
	add("FAMILY", deref(s.Family))
	if k := deref(s.Kconfig); k != "" {
		add("KCONFIG", k+" (may return ENOSYS if disabled)")
	}
	pairs = append(pairs, [2]string{"ENTRY POINT", s.Entry})
	if s.Man != nil {
		pairs = append(pairs, [2]string{"MAN PAGE", *s.Man})
	}
	add("SOURCE", s.Source)
	var b strings.Builder
	for _, p := range pairs {
		for i, line := range wrapText(p[1], d.width-14) {
			label := strings.Repeat(" ", 12)
			if i == 0 {
				label = d.paint(fmt.Sprintf("%-12s", p[0]), "1;36")
			}
			b.WriteString(label + "  " + line + "\n")
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func (d display) conventions(out io.Writer, cs []convention) error {
	rows := [][]string{}
	for _, c := range cs {
		rows = append(rows, []string{c.Arch, c.Instruction, c.Nr, c.Return, strings.Join(c.registers(), ", ")})
	}
	if err := d.table(out, []string{"ARCH", "INSTRUCTION", "NUMBER", "RETURN", "ARGUMENTS"}, rows); err != nil {
		return err
	}
	var b strings.Builder
	for _, c := range cs {
		if len(c.Notes) == 0 && c.Until == "" {
			continue
		}
		b.WriteString("\n" + d.paint("ABI notes · "+c.Arch, "1;36") + "\n")
		if c.Until != "" {
			b.WriteString("  - Architecture removed in Linux " + c.Until + "\n")
		}
		for _, note := range c.Notes {
			for i, line := range wrapText(note, d.width-4) {
				prefix := "  - "
				if i > 0 {
					prefix = "    "
				}
				b.WriteString(prefix + line + "\n")
			}
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func nargs(s syscall) int {
	if s.Nargs == nil {
		return 0
	}
	return *s.Nargs
}

// card prints each syscall name once with its signature, then one row per
// arch with the number and the registers needed to make the call.
func (d display) card(out io.Writer, rows []syscall, hidden int) error {
	var names []string
	byName := map[string][]syscall{}
	for _, s := range rows {
		if byName[s.Name] == nil {
			names = append(names, s.Name)
		}
		byName[s.Name] = append(byName[s.Name], s)
	}
	for i, name := range names {
		group := byName[name]
		first := group[0]
		var b strings.Builder
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.paint(name, "1;36"))
		if desc := deref(first.Description); desc != "" {
			b.WriteString(" - " + desc)
		}
		b.WriteByte('\n')
		// Most syscalls share one signature everywhere, the odd ones list their arches.
		var sigs []string
		sigArches := map[string][]string{}
		width := 0
		for _, s := range group {
			sig := signature(s)
			if sigArches[sig] == nil {
				sigs = append(sigs, sig)
			}
			sigArches[sig] = append(sigArches[sig], s.Arch)
			width = max(width, nargs(s))
		}
		for _, sig := range sigs {
			b.WriteString(sig)
			if len(sigs) > 1 {
				b.WriteString("  " + d.paint("on "+strings.Join(sigArches[sig], ", "), "2"))
			}
			b.WriteByte('\n')
		}
		if k := deref(first.Kconfig); k != "" {
			b.WriteString(d.paint("needs "+k+", returns ENOSYS without it", "2") + "\n")
		}
		b.WriteByte('\n')
		if _, err := io.WriteString(out, b.String()); err != nil {
			return err
		}

		headers := []string{"ARCH", "NR", "HEX", "INSN", "NR REG"}
		for j := range width {
			headers = append(headers, fmt.Sprintf("ARG%d", j))
		}
		headers = append(headers, "RET")
		removed := slices.ContainsFunc(group, func(s syscall) bool { return s.Removed != nil })
		if removed {
			headers = append(headers, "REMOVED")
		}
		var table [][]string
		for _, s := range group {
			c, _ := conventionFor(s.Arch)
			row := []string{s.Arch, d.paint(strconv.Itoa(s.Nr), "1"), d.paint(fmt.Sprintf("%#x", s.Nr), "1"), orDash(c.Instruction), orDash(c.Nr)}
			for j, reg := range c.registers()[:width] {
				if j >= nargs(s) {
					reg = ""
				}
				row = append(row, orDash(reg))
			}
			row = append(row, orDash(c.Return))
			if removed {
				row = append(row, dash(s.Removed))
			}
			table = append(table, row)
		}
		if err := d.table(out, headers, table); err != nil {
			return err
		}
	}
	if hidden > 0 {
		_, err := fmt.Fprintf(out, "\n%s\n", d.paint(fmt.Sprintf("%d more arches with --arch all", hidden), "2"))
		return err
	}
	return nil
}
