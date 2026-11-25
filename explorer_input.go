package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m explorer) listWidth() int {
	if m.width >= 76 {
		return max(34, min(42, m.width*42/100))
	}
	return m.width - 2
}

func wheelDelta(b tea.MouseButton) int {
	switch b {
	case tea.MouseWheelUp:
		return -3
	case tea.MouseWheelDown:
		return 3
	}
	return 0
}

func (m explorer) updateMouseWheel(v tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	d := wheelDelta(v.Button)
	switch {
	case m.palette:
	case m.showHelp:
		m.helpOffset = max(0, m.helpOffset+d)
	case (m.width >= 76 && v.X >= m.listWidth()) || (m.width < 76 && m.showDetails):
		m.detailOffset = max(0, m.detailOffset+d)
	default:
		m.move(d)
		m.detailOffset = 0
	}
	return m, nil
}

func (m explorer) updateMouseClick(v tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.palette || m.showHelp || m.width < 40 || m.height < 10 || v.Button != tea.MouseLeft {
		return m, nil
	}
	// The list starts on row 5, below the title, search, rule, count and column header.
	if v.X < m.listWidth() && v.Y >= 5 && v.Y < m.height-2 && (m.width >= 76 || !m.showDetails) {
		if idx := m.listStart(m.height-7) + v.Y - 5; idx < len(m.matches) {
			m.cursor = idx
			m.detailOffset = 0
		}
	}
	return m, nil
}

func (m explorer) updatePaste(v tea.PasteMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.palette:
		m.appendPaletteQuery(v.Content)
	case m.showHelp:
	case m.fieldMode != "":
		m.editField("", v.Content)
	case m.searching:
		m.editSearch("", v.Content)
	}
	return m, nil
}

func (m *explorer) startField(mode, value, status string) {
	m.fieldMode, m.fieldInput, m.status = mode, value, status
	m.fieldCursor = len([]rune(value))
}

func (m explorer) updateKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := v.String()
	switch {
	case key == "ctrl+c":
		return m, tea.Quit
	case m.palette:
		return m.paletteUpdate(key, v.Text)
	case m.showHelp:
		return m.updateHelpKey(key)
	case m.fieldMode != "":
		return m.updateFieldKey(key, v.Text)
	case m.searching:
		return m.updateSearchKey(key, v.Text)
	}
	return m.dispatchKey(key)
}

// dispatchKey runs a key in the default mode, after any modal closed.
func (m explorer) dispatchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		m.helpOffset = 0
	case ":":
		m.palette = true
		m.paletteQuery = ""
		m.paletteIndex = 0
	case "/":
		m.detailsBeforeSearch = m.showDetails
		m.showDetails = false
		m.searching = true
		m.inputCursor = len([]rune(m.input))
	case "A":
		m.startField("architecture", m.arch, "Type an architecture, or leave it empty for all")
	case "K":
		m.startField("kernel", m.kernel, "Type a kernel release, empty for latest or all for history")
	case "D":
		m.startField("diff", m.diffFrom+" "+m.diffTo, "Type two kernel releases")
	case "[":
		m.showAction(m.shiftKernel(-1), "Kernel changed")
	case "]":
		m.showAction(m.shiftKernel(1), "Kernel changed")
	case "C":
		m.showAction(copyText(m.shareCommand()), "Copied explorer command")
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if m.viewMode == "related" || (m.viewMode == "" && m.width >= 140 && !m.hideSummary) {
			m.jumpRelated(int(key[0] - '0'))
		}
	case "i":
		m.hideSummary = !m.hideSummary
	case "f":
		m.favoritesOnly = !m.favoritesOnly
		m.filter()
		m.status = "Showing all syscalls"
		if m.favoritesOnly {
			m.status = "Showing favorites"
		}
	case "enter":
		if m.width < 76 && len(m.matches) > 0 {
			m.showDetails = true
			m.detailOffset = 0
		}
	case "esc":
		m.viewMode = ""
		m.showDetails = false
		m.detailOffset = 0
	case "up", "k", "ctrl+p":
		m.move(-1)
	case "down", "j", "ctrl+n":
		m.move(1)
	case "pgdown", "ctrl+v":
		m.detailOffset += 5
	case "pgup", "alt+v":
		m.detailOffset = max(0, m.detailOffset-5)
	case "home":
		m.cursor = 0
		m.detailOffset = 0
	case "end":
		m.cursor = max(0, len(m.matches)-1)
		m.detailOffset = 0
	default:
		if s := m.selected(); s != nil {
			return m.updateSelectionKey(key, *s)
		}
	}
	return m, nil
}

// updateSelectionKey handles keys that act on the selected syscall.
func (m explorer) updateSelectionKey(key string, s syscall) (tea.Model, tea.Cmd) {
	switch key {
	case "c":
		m.toggleDetailView("compare")
	case "t":
		m.toggleDetailView("timeline")
	case "r":
		m.toggleDetailView("related")
	case "a":
		m.toggleDetailView("asm")
	case "g":
		m.showAction(m.setFilters(s.Arch, s.Since), "Jumped to first scanned release")
	case "G":
		if s.Removed == nil {
			m.status = "No removal recorded"
		} else {
			m.showAction(m.setFilters(s.Arch, *s.Removed), "Jumped to removal release, the syscall is gone here")
		}
	case "Y":
		text, err := assemblySnippet(s)
		if err == nil {
			err = copyText(text)
		}
		m.showAction(err, "Copied assembly snippet")
	case "y":
		if s.Nargs == nil {
			m.status = "Signature unavailable"
		} else {
			m.showAction(copyText(signature(s)), "Copied signature")
		}
	case "m":
		if s.Man == nil {
			m.status = "No man page available"
		} else {
			m.showAction(openReference(*s.Man), "Opened man page")
		}
	case "M":
		key := favoriteKey(s)
		if m.viewMode == "man" && m.manKey == key {
			m.viewMode, m.detailOffset = "", 0
			return m, nil
		}
		m.viewMode, m.showDetails, m.detailOffset = "man", true, 0
		m.manKey, m.manText, m.manLoading = key, "", true
		return m, loadManPage(key, s.Name)
	case "s":
		m.showAction(openReference(s.Source), "Opened kernel source")
	case "*":
		next, err := toggleFavorite(favoriteKey(s))
		if err != nil {
			m.status = err.Error()
			break
		}
		m.favorites = next
		m.status = "Favorites saved"
		if m.favoritesOnly {
			m.filter()
		}
	}
	return m, nil
}

func (m *explorer) toggleDetailView(view string) {
	if m.viewMode == view {
		m.viewMode = ""
	} else {
		m.viewMode = view
	}
	m.showDetails = true
	m.detailOffset = 0
}

func (m explorer) updateHelpKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k", "ctrl+p":
		m.helpOffset = max(0, m.helpOffset-1)
	case "down", "j", "ctrl+n":
		m.helpOffset++
	case "pgup", "alt+v":
		m.helpOffset = max(0, m.helpOffset-5)
	case "pgdown", "ctrl+v":
		m.helpOffset += 5
	case "home":
		m.helpOffset = 0
	default:
		m.showHelp = false
		m.helpOffset = 0
	}
	return m, nil
}

func (m explorer) updateFieldKey(key, text string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "ctrl+g":
		m.fieldMode, m.status = "", ""
	case "enter":
		if m.fieldMode == "diff" {
			m.applyDiff()
			break
		}
		arch, kernel := m.arch, m.kernel
		if m.fieldMode == "architecture" {
			arch = strings.TrimSpace(m.fieldInput)
		} else {
			kernel = strings.TrimSpace(m.fieldInput)
		}
		if err := m.setFilters(arch, kernel); err != nil {
			m.status = err.Error()
		} else {
			m.fieldMode, m.status = "", "Filters updated"
		}
	default:
		m.editField(key, text)
	}
	return m, nil
}

func (m *explorer) applyDiff() {
	parts := strings.Fields(m.fieldInput)
	if len(parts) != 2 {
		m.status = "Need two releases separated by a space"
		return
	}
	arch := m.arch
	if s := m.selected(); arch == "" && s != nil {
		arch = s.Arch
	}
	if arch == "" {
		m.status = "Select a syscall or set an architecture"
		return
	}
	if _, err := diffKernels(parts[0], parts[1], arch); err != nil {
		m.status = err.Error()
		return
	}
	m.diffFrom, m.diffTo, m.diffArch = parts[0], parts[1], arch
	m.fieldMode, m.status = "", ""
	m.viewMode, m.showDetails, m.detailOffset = "diff", true, 0
}

func (m explorer) updateSearchKey(key, text string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		if len(m.matches) > 0 {
			m.input = m.all[m.matches[0]].Name
			m.inputCursor = len([]rune(m.input))
			m.filter()
		}
	case "esc", "enter", "ctrl+g":
		// Bring back the compact detail pane that / hid.
		m.searching = false
		m.showDetails = m.detailsBeforeSearch && len(m.matches) > 0
	case "down", "ctrl+n":
		m.move(1)
	case "up", "ctrl+p":
		m.move(-1)
	default:
		m.editSearch(key, text)
	}
	return m, nil
}
