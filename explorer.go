package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

type explorer struct {
	all                                     []syscall
	matches                                 []int
	input                                   string
	inputCursor                             int
	killed                                  string
	cursor, detailOffset                    int
	width, height                           int
	searching, showDetails                  bool
	arch, kernel                            string
	viewMode, fieldMode, fieldInput, status string
	diffFrom, diffTo, diffArch              string
	fieldCursor                             int
	helpOffset                              int
	favorites                               map[string]bool
	favoritesOnly, showHelp                 bool
	theme                                   string
	palette                                 bool
	paletteQuery                            string
	paletteIndex                            int
	hideSummary                             bool
	manKey, manText                         string
	manLoading                              bool
	detailsBeforeSearch                     bool
	archNumbers                             map[string]map[string]int
}

func newExplorer(arch, kernel string) (explorer, error) {
	arch = canonicalArch(arch)
	d, err := loadDataset()
	if err != nil {
		return explorer{}, err
	}
	keep, err := d.filter(arch, kernel)
	if err != nil {
		return explorer{}, err
	}
	m := explorer{width: 100, height: 30, arch: arch, kernel: kernel, all: d.collect(keep), archNumbers: d.archNumbers(kernel)}
	m.filter()
	return m, nil
}

func (m *explorer) filter() {
	m.matches = rank(m.all, m.input, func(s syscall) bool { return !m.favoritesOnly || m.favorites[favoriteKey(s)] })
	m.cursor = 0
	m.detailOffset = 0
}

// editSearch applies an editing key, or inserts text when key is empty.
func (m *explorer) editSearch(key, text string) {
	before := m.input
	ed := textEditor{value: m.input, cursor: m.inputCursor, killed: m.killed}
	ed.edit(key, text)
	m.input, m.inputCursor, m.killed = ed.value, ed.cursor, ed.killed
	if m.input != before {
		m.filter()
	}
}

func (m *explorer) editField(key, text string) {
	ed := textEditor{value: m.fieldInput, cursor: m.fieldCursor, killed: m.killed}
	ed.edit(key, text)
	m.fieldInput, m.fieldCursor, m.killed = ed.value, ed.cursor, ed.killed
}

func (m *explorer) selectKey(key string) bool {
	for i, index := range m.matches {
		if favoriteKey(m.all[index]) == key {
			m.cursor = i
			return true
		}
	}
	return false
}

func (m *explorer) setFilters(arch, kernel string) error {
	arch = canonicalArch(arch)
	n, err := newExplorer(arch, kernel)
	if err != nil {
		return err
	}
	key := ""
	if s := m.selected(); s != nil {
		key = favoriteKey(*s)
	}
	m.arch, m.kernel, m.all, m.archNumbers = arch, kernel, n.all, n.archNumbers
	m.filter()
	m.selectKey(key)
	return nil
}

func (m *explorer) selected() *syscall {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return nil
	}
	return &m.all[m.matches[m.cursor]]
}

// move shifts the selection, resetting the detail scroll when it changes.
func (m *explorer) move(delta int) {
	if next := max(0, min(m.cursor+delta, len(m.matches)-1)); next != m.cursor {
		m.cursor, m.detailOffset = next, 0
	}
}

// listStart is shared by rendering and mouse hit testing so both agree on
// which row is on screen once the selection scrolls past the viewport.
func (m explorer) listStart(height int) int {
	return max(0, m.cursor-height+1)
}

func (m *explorer) showAction(err error, success string) {
	if err != nil {
		m.status = err.Error()
	} else {
		m.status = success
	}
}

func (m *explorer) shiftKernel(step int) error {
	d, err := loadDataset()
	if err != nil {
		return err
	}
	versions := d.releases(m.arch)
	if len(versions) == 0 {
		return errors.New("no kernel releases available")
	}
	idx := len(versions) - 1
	if m.kernel != "" && m.kernel != "all" {
		current, err := parseVersion(m.kernel)
		if err != nil {
			return err
		}
		if i := slices.Index(versions, current); i >= 0 {
			idx = i
		}
	}
	return m.setFilters(m.arch, versions[max(0, min(len(versions)-1, idx+step))].String())
}

func (m explorer) Init() tea.Cmd { return nil }

func (m explorer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case tea.MouseWheelMsg:
		return m.updateMouseWheel(v)
	case tea.MouseClickMsg:
		return m.updateMouseClick(v)
	case tea.PasteMsg:
		return m.updatePaste(v)
	case tea.KeyPressMsg:
		return m.updateKey(v)
	case manPageResult:
		if v.key == m.manKey {
			m.manText, m.manLoading = v.text, false
		}
	}
	return m, nil
}

// explorerViews are the initial views the explore command accepts.
var explorerViews = []string{"details", "compare", "timeline", "related", "diff", "asm"}

// explorerStart holds the explore flags.
type explorerStart struct {
	arch, kernel, query, selected, view, theme string
	diffFrom, diffTo, diffArch                 string
	favorites, details                         bool
}

func runExplorer(out io.Writer, start explorerStart) error {
	// Diff flags only make sense in the diff view, so they imply it.
	diffSet := start.diffFrom != "" || start.diffTo != "" || start.diffArch != ""
	if diffSet && start.view == "details" {
		start.view = "diff"
	}
	if diffSet && start.view != "diff" {
		return errors.New("--diff-from, --diff-to, and --diff-arch need the diff view")
	}
	if !slices.Contains(explorerViews, start.view) {
		return fmt.Errorf("view must be details, compare, timeline, related, diff, or asm")
	}
	if err := applyTheme(start.theme); err != nil {
		return err
	}
	f, ok := out.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("explorer requires an interactive terminal")
	}
	m, err := newExplorer(start.arch, start.kernel)
	if err != nil {
		return err
	}
	if m.favorites, err = readFavorites(); err != nil {
		m.status = err.Error()
	}
	m.theme = start.theme
	// Go through the editor so the query gets the same cap and cleanup as typing.
	ed := textEditor{}
	ed.insert(start.query)
	m.input, m.inputCursor = ed.value, ed.cursor
	m.favoritesOnly = start.favorites
	m.filter()
	if start.selected != "" && !m.selectKey(start.selected) {
		return fmt.Errorf("selected syscall is not in the filtered results")
	}
	if start.view == "diff" {
		arch := canonicalArch(start.diffArch)
		if start.diffFrom == "" || start.diffTo == "" || arch == "" {
			return errors.New("diff view requires --diff-from, --diff-to, and --diff-arch")
		}
		if _, err := diffKernels(start.diffFrom, start.diffTo, arch); err != nil {
			return err
		}
		m.diffFrom, m.diffTo, m.diffArch = start.diffFrom, start.diffTo, arch
	}
	m.showDetails = start.details
	if start.view != "details" {
		m.viewMode = start.view
		m.showDetails = true
	}
	_, err = tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(out)).Run()
	return err
}
