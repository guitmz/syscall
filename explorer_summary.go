package main

import "fmt"

// summaryLines fills the extra side panel shown on wide terminals.
func (m explorer) summaryLines(width int) []string {
	s := m.selected()
	if s == nil {
		return nil
	}
	var lines []string
	add := func(text ...string) { lines = append(lines, text...) }
	if m.viewMode == "asm" {
		add(tuiSection.Render("REGISTERS"))
		c, err := conventionFor(s.Arch)
		if err != nil {
			add("Convention unavailable")
			return wrapAll(lines, width)
		}
		add("Number  "+c.Nr, "Return  "+c.Return)
		regs := c.registers()
		if s.Nargs != nil {
			for i := 0; i < min(max(*s.Nargs, 0), len(regs)); i++ {
				arg := deref(s.args()[i])
				if arg == "" {
					arg = "unknown argument"
				}
				add(regs[i] + "  " + arg)
			}
		}
		return wrapAll(lines, width)
	}
	add(tuiSection.Render("ARCHITECTURES"))
	numbers := m.archNumbers[s.Name]
	for _, arch := range []string{"x64", "x86", "arm64", "arm", "riscv64"} {
		if nr, ok := numbers[arch]; ok {
			add(fmt.Sprintf("%-8s %d", arch, nr))
		}
	}
	end := "latest"
	if s.Removed != nil {
		end = "removed " + *s.Removed
	}
	add("", tuiSection.Render("KERNEL TIMELINE"), s.Since+"  ━━━━━  "+end)
	add("", tuiSection.Render("RELATED · 1 to 9"))
	indices := m.relatedIndices()
	if len(indices) == 0 {
		add("None in this view")
	}
	for i, index := range indices {
		add(fmt.Sprintf("%d  %s", i+1, m.all[index].Name))
	}
	return wrapAll(lines, width)
}

// archNumbers indexes syscall numbers by name then architecture under a
// kernel filter, so the wide summary panel doesn't rescan the dataset.
func archNumbers(kernel string) map[string]map[string]int {
	d, err := loadDataset()
	if err != nil {
		return nil
	}
	keep, err := d.filter("", kernel)
	if err != nil {
		return nil
	}
	index := map[string]map[string]int{}
	for _, s := range d.syscalls {
		if !keep(s) {
			continue
		}
		if index[s.Name] == nil {
			index[s.Name] = map[string]int{}
		}
		if _, exists := index[s.Name][s.Arch]; !exists {
			index[s.Name][s.Arch] = s.Nr
		}
	}
	return index
}
