package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	tuiAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	tuiMuted    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	tuiText     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	tuiSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).Bold(true)
	tuiSection  = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
)

// fit truncates or pads s to exactly width cells.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

// wrapAll wraps every line to width.
func wrapAll(lines []string, width int) []string {
	var out []string
	for _, line := range lines {
		out = append(out, wrapText(line, width)...)
	}
	return out
}

func detailLines(s syscall, width int) []string {
	var lines []string
	add := func(text ...string) { lines = append(lines, text...) }
	add(tuiAccent.Render(s.Name) + "  " + tuiMuted.Render(fmt.Sprintf("%s · #%d", s.Arch, s.Nr)))
	if d := deref(s.Description); d != "" {
		add(d)
	} else {
		add(tuiMuted.Render("No description available"))
	}
	add("", tuiSection.Render("SIGNATURE"))
	if s.Nargs == nil {
		add(tuiMuted.Render("Not available"))
	} else {
		add(signature(s))
	}
	add("", tuiSection.Render("KERNEL & ABI"))
	add("Architecture  "+s.Arch+"   ABI  "+s.ABI, "Since         "+s.Since)
	if f := deref(s.Family); f != "" {
		add("Family        " + f)
	}
	if s.Removed != nil {
		add("Removed       " + *s.Removed)
	}
	if s.Entry != "" {
		add("Entry point   " + s.Entry)
	}
	if k := deref(s.Kconfig); k != "" {
		add("Kconfig       "+k, tuiMuted.Render("If disabled, the syscall may return ENOSYS."))
	}
	add("", tuiSection.Render("CALLING CONVENTION"))
	if c, err := conventionFor(s.Arch); err == nil {
		if c.Until != "" {
			add("Architecture removed in Linux " + c.Until)
		}
		add("Instruction   "+c.Instruction, "Number        "+c.Nr, "Return        "+c.Return)
		regs := c.registers()
		if s.Nargs != nil {
			for i := 0; i < min(*s.Nargs, len(regs)); i++ {
				value := "?"
				if a := s.args()[i]; a != nil {
					value = *a
				}
				add(fmt.Sprintf("  arg%d  %-7s → %s", i, regs[i], value))
			}
		} else {
			add("Arguments     " + strings.Join(regs, ", "))
		}
		if len(c.Notes) > 0 {
			add("")
			for _, note := range c.Notes {
				add("• " + note)
			}
		}
	} else {
		add(tuiMuted.Render("Not available"))
	}
	add("", tuiSection.Render("REFERENCES"))
	if man := deref(s.Man); man != "" {
		add("Man     " + man)
	} else {
		add(tuiMuted.Render("Man     Not available"))
	}
	if s.Source != "" {
		add("Source  " + s.Source)
	}
	return wrapAll(lines, width)
}

// window returns the visible slice of lines and the clamped scroll offset.
func window(lines []string, offset, height int) ([]string, int) {
	offset = min(offset, max(0, len(lines)-height))
	return lines[offset:min(len(lines), offset+height)], offset
}

func (m explorer) searchPrompt(width int) string {
	if !m.searching {
		if m.input == "" {
			return tuiMuted.Render("SEARCH  ") + tuiDim.Render("name, number, architecture, family, description")
		}
		return tuiMuted.Render("SEARCH  ") + m.input
	}
	r := []rune(m.input)
	p := min(max(0, m.inputCursor), len(r))
	before, after := string(r[:p]), r[p:]
	// Keep the caret on screen as the query grows.
	if available := max(1, width-10); lipgloss.Width(before) >= available {
		before = ansi.Cut(before, lipgloss.Width(before)-available+1, lipgloss.Width(before))
	}
	caret := tuiAccent.Render("▏")
	if len(after) > 0 {
		caret = lipgloss.NewStyle().Reverse(true).Render(string(after[0]))
		after = after[1:]
	}
	return tuiAccent.Render("SEARCH  ") + before + caret + string(after)
}

func (m explorer) View() tea.View {
	w, h := m.width, m.height
	if w < 40 || h < 10 {
		v := tea.NewView("syscall explorer\n\nResize terminal to at least 40 × 10.\n\nq quit")
		v.AltScreen = true
		return v
	}
	scope, release := m.arch, m.kernel
	if scope == "" {
		scope = "all architectures"
	}
	if release == "" {
		release = "latest"
	}
	top := tuiAccent.Render("◆ SYSCALL") + tuiText.Render("  /  EXPLORER") + "   " + tuiMuted.Render(scope+" · "+release)
	prompt := m.searchPrompt(w)

	count := fmt.Sprintf("%d MATCHES", len(m.matches))
	if m.favoritesOnly {
		count += " · FAVORITES"
	}
	pos := ""
	if len(m.matches) > 0 {
		pos = fmt.Sprintf("%d / %d", m.cursor+1, len(m.matches))
	}
	compact := w < 76
	listWidth := m.listWidth()
	rightWidth, summaryWidth := 0, 0
	if !compact {
		rightWidth = w - listWidth - 3
	}
	if w >= 140 && !m.hideSummary && (m.viewMode == "" || m.viewMode == "asm") {
		summaryWidth = 32
	}
	detailWidth := rightWidth
	if summaryWidth > 0 {
		detailWidth -= summaryWidth + 3
	}
	bodyHeight := h - 7
	sep := tuiDim.Render(" │ ")

	archWidth, numberWidth := 4, 2
	for _, i := range m.matches {
		archWidth = max(archWidth, lipgloss.Width(m.all[i].Arch))
		numberWidth = max(numberWidth, len(fmt.Sprint(m.all[i].Nr)))
	}
	archWidth = min(archWidth, max(4, listWidth-numberWidth-10))
	nameWidth := max(1, listWidth-archWidth-numberWidth-6)
	columnLine := func(marker, name, arch, number string) string {
		return marker + fit(name, nameWidth) + "  " + fmt.Sprintf("%*s", archWidth, ansi.Truncate(arch, archWidth, "…")) + "  " + fmt.Sprintf("%*s", numberWidth, number)
	}
	columns := tuiMuted.Render(columnLine("  ", "NAME", "ARCH", "NR"))
	if !compact {
		columns += sep + strings.Repeat(" ", rightWidth)
	}

	var left []string
	for i := m.listStart(bodyHeight); i < len(m.matches) && len(left) < bodyHeight; i++ {
		s := m.all[m.matches[i]]
		if i == m.cursor {
			left = append(left, tuiSelected.Render(fit(columnLine("› ", s.Name, s.Arch, fmt.Sprint(s.Nr)), listWidth)))
			continue
		}
		marker := "  "
		if m.favorites[favoriteKey(s)] {
			marker = "★ "
		}
		left = append(left, tuiText.Render(fit(columnLine(marker, s.Name, s.Arch, fmt.Sprint(s.Nr)), listWidth)))
	}
	if len(m.matches) == 0 {
		left = append(left, tuiMuted.Render("  No matching syscalls"))
	}

	heading := tuiMuted.Render(fit("  "+count, listWidth))
	var right, summary []string
	if !compact {
		right = []string{tuiMuted.Render("No matching syscalls.")}
		if len(m.matches) > 0 {
			right = m.currentDetail(max(1, detailWidth-2))
		}
		all := len(right)
		var offset int
		right, offset = window(right, m.detailOffset, bodyHeight)
		progress := ""
		if all > bodyHeight {
			progress = fmt.Sprintf("  ↑%d ↓%d", offset, max(0, all-offset-bodyHeight))
		}
		heading += sep + tuiMuted.Render(fit("DETAILS  "+pos+progress, rightWidth))
		if summaryWidth > 0 {
			summary = m.summaryLines(summaryWidth - 2)
		}
	}
	cell := func(lines []string, i int, pad string) string {
		if i < len(lines) {
			return pad + lines[i]
		}
		return ""
	}
	var body strings.Builder
	for i := 0; i < bodyHeight; i++ {
		body.WriteString(fit(cell(left, i, ""), listWidth))
		if !compact {
			body.WriteString(sep + fit(cell(right, i, "  "), detailWidth))
			if summaryWidth > 0 {
				body.WriteString(sep + fit(cell(summary, i, " "), summaryWidth))
			}
		}
		body.WriteByte('\n')
	}

	footer := tuiMuted.Render("/ search  : commands  c compare  t timeline  ? help  q quit")
	if m.searching {
		hint := "C-a/e start/end  C-b/f move  C-w/u/k kill  enter done"
		if len(m.matches) > 0 && m.input != "" {
			hint = "tab → " + m.all[m.matches[0]].Name + "  ·  " + hint
		}
		footer = tuiMuted.Render(hint)
	} else if compact {
		footer = tuiMuted.Render("/ search  ↑↓/C-n/p move  enter details  q quit")
	}
	if compact && m.showDetails && len(m.matches) > 0 {
		d := m.currentDetail(w - 2)
		visible, offset := window(d, m.detailOffset, bodyHeight)
		body.Reset()
		for i := 0; i < bodyHeight; i++ {
			body.WriteString(fit(cell(visible, i, "  "), w) + "\n")
		}
		heading = tuiMuted.Render(fmt.Sprintf("  DETAILS  %s  ↑%d ↓%d", pos, offset, max(0, len(d)-offset-bodyHeight)))
		columns = ""
		footer = tuiMuted.Render("esc back  pgup/pgdn scroll  q quit")
	}

	if m.fieldMode != "" {
		prompt = tuiAccent.Render(strings.ToUpper(m.fieldMode)+"  ") + m.fieldInput + tuiAccent.Render("▏")
		footer = tuiMuted.Render("enter apply  esc cancel  C-a/e start/end")
		if m.status != "" {
			footer = tuiMuted.Render(m.status + "  ·  enter apply  esc cancel")
		}
	} else if m.status != "" && !m.searching {
		footer = tuiAccent.Render(m.status) + tuiMuted.Render("  ·  ? help")
	}
	content := strings.Join([]string{fit(top, w), fit(prompt, w), tuiDim.Render(strings.Repeat("─", w)), fit(heading, w), fit(columns, w)}, "\n") + "\n" + body.String() + fit(footer, w)
	if m.showHelp {
		content = helpModal(content, w, h, m.helpOffset)
	}
	if m.palette {
		content = m.paletteModal(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m explorer) currentDetail(width int) []string {
	s := m.selected()
	if s == nil {
		return []string{"No matching syscalls."}
	}
	switch m.viewMode {
	case "compare":
		return compareLines(*s, m.kernel, width)
	case "timeline":
		return timelineLines(*s, width)
	case "related":
		return m.relatedLines(width)
	case "diff":
		return diffLines(m.diffFrom, m.diffTo, m.diffArch, width)
	case "asm":
		code, err := assemblySnippet(*s)
		if err != nil {
			return []string{err.Error()}
		}
		lines := []string{tuiSection.Render("ASSEMBLY SNIPPET"), "", tuiMuted.Render("Y copies plain text"), ""}
		for _, line := range strings.Split(strings.TrimSuffix(code, "\n"), "\n") {
			style := tuiAccent
			if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
				style = tuiMuted
			}
			for _, part := range wrapText(line, width) {
				lines = append(lines, style.Render(part))
			}
		}
		return lines
	case "man":
		if favoriteKey(*s) != m.manKey {
			return []string{"Press M to load the local man page for this syscall."}
		}
		if m.manLoading {
			return []string{"Loading the local man page…"}
		}
		return wrapAll(strings.Split(strings.TrimSuffix(m.manText, "\n"), "\n"), width)
	default:
		return detailLines(*s, width)
	}
}
