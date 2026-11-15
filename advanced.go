package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// syscallNumber parses decimal or 0x hex. ok is false for anything that isn't
// a number, and a broken hex value is an error.
func syscallNumber(s string) (int, bool, error) {
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		n, e := strconv.ParseUint(s[2:], 16, 63)
		if e != nil || uint64(int(n)) != n {
			return 0, true, fmt.Errorf("invalid syscall number %q", s)
		}
		return int(n), true, nil
	}
	if n, e := strconv.Atoi(s); e == nil {
		return n, true, nil
	}
	return 0, false, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func (m explorer) shareCommand() string {
	parts := []string{"syscall", "explore"}
	add := func(flag, value string) {
		if value != "" {
			parts = append(parts, flag, shellQuote(value))
		}
	}
	add("--arch", m.arch)
	add("--kernel", m.kernel)
	add("--query", m.input)
	if m.favoritesOnly {
		parts = append(parts, "--favorites")
	}
	if m.theme != "auto" {
		add("--theme", m.theme)
	}
	if s := m.selected(); s != nil {
		add("--select", favoriteKey(*s))
	}
	if m.showDetails && m.viewMode == "" {
		parts = append(parts, "--open-details")
	}
	if m.viewMode == "diff" {
		add("--diff-from", m.diffFrom)
		add("--diff-to", m.diffTo)
		add("--diff-arch", m.diffArch)
	}
	if m.viewMode != "" && m.viewMode != "man" {
		add("--view", m.viewMode)
	}
	return strings.Join(parts, " ")
}

func resolveTheme(theme string, autoMono bool) (string, error) {
	if theme == "auto" {
		if autoMono {
			theme = "mono"
		} else {
			theme = "dark"
		}
	}
	if theme != "dark" && theme != "light" && theme != "mono" {
		return "", errors.New("theme must be auto, dark, light, or mono")
	}
	return theme, nil
}

func applyTheme(theme string) error {
	theme, err := resolveTheme(theme, os.Getenv("NO_COLOR") != "")
	if err != nil {
		return err
	}
	if theme == "mono" {
		tuiAccent = lipgloss.NewStyle().Bold(true)
		tuiMuted, tuiDim, tuiText = lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle()
		tuiSelected = lipgloss.NewStyle().Reverse(true)
		tuiSection = lipgloss.NewStyle().Bold(true)
		return nil
	}
	// accent, muted, dim, text, section
	c := map[string][5]string{
		"dark":  {"81", "245", "240", "252", "75"},
		"light": {"25", "240", "245", "235", "25"},
	}[theme]
	fg := func(color string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(color)) }
	tuiAccent = fg(c[0]).Bold(true)
	tuiMuted, tuiDim, tuiText = fg(c[1]), fg(c[2]), fg(c[3])
	tuiSelected = fg("231").Background(lipgloss.Color("24")).Bold(true)
	tuiSection = fg(c[4]).Bold(true)
	return nil
}
