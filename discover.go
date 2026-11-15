package main

import (
	"fmt"
	"slices"
)

// relatedGroups backs the related view for syscalls without a family.
var relatedGroups = [][]string{
	{"read", "write", "pread64", "pwrite64", "readv", "writev", "preadv", "pwritev", "preadv2", "pwritev2"},
	{"open", "openat", "openat2", "creat", "close", "close_range"},
	{"socket", "socketpair", "bind", "listen", "connect", "accept", "accept4", "sendto", "recvfrom", "sendmsg", "recvmsg"},
	{"stat", "lstat", "fstat", "newfstatat", "statx"},
	{"fork", "vfork", "clone", "clone3", "execve", "execveat", "wait4", "waitid"},
	{"mmap", "mprotect", "munmap", "mremap", "madvise"},
	{"poll", "ppoll", "select", "pselect6", "epoll_create1", "epoll_ctl", "epoll_wait", "epoll_pwait", "epoll_pwait2"},
	{"io_uring_setup", "io_uring_enter", "io_uring_register"},
}

// relatedIndices returns up to nine syscalls on the same architecture, taken
// from the selected syscall's family or else its curated group.
func (m explorer) relatedIndices() []int {
	s := m.selected()
	if s == nil {
		return nil
	}
	var indices []int
	if family := deref(s.Family); family != "" {
		seen := map[string]bool{s.Name: true}
		for i, c := range m.all {
			if c.Arch == s.Arch && deref(c.Family) == family && !seen[c.Name] {
				seen[c.Name] = true
				indices = append(indices, i)
				if len(indices) == 9 {
					break
				}
			}
		}
		return indices
	}
	i := slices.IndexFunc(relatedGroups, func(g []string) bool { return slices.Contains(g, s.Name) })
	if i < 0 {
		return nil
	}
	for _, name := range relatedGroups[i] {
		if name == s.Name {
			continue
		}
		if j := slices.IndexFunc(m.all, func(c syscall) bool { return c.Name == name && c.Arch == s.Arch }); j >= 0 {
			indices = append(indices, j)
			if len(indices) == 9 {
				break
			}
		}
	}
	return indices
}

func (m explorer) relatedLines(width int) []string {
	s := m.selected()
	if s == nil {
		return []string{"No selected syscall"}
	}
	source := "Curated group · press 1 to 9 to jump"
	if family := deref(s.Family); family != "" {
		source = "Family: " + family + " · press 1 to 9 to jump"
	}
	lines := []string{tuiSection.Render("RELATED SYSCALLS"), s.Name + " · " + s.Arch, "", tuiMuted.Render(source), ""}
	indices := m.relatedIndices()
	if len(indices) == 0 {
		return append(lines, "No related syscalls in this view.")
	}
	for i, index := range indices {
		r := m.all[index]
		lines = append(lines, fmt.Sprintf("%d  %-18s  #%d", i+1, r.Name, r.Nr))
	}
	return wrapAll(lines, width)
}

func (m *explorer) jumpRelated(n int) {
	indices := m.relatedIndices()
	if n < 1 || n > len(indices) {
		return
	}
	// Never wipe the user's search or favorites filter just to jump.
	target := m.all[indices[n-1]]
	if !m.selectKey(favoriteKey(target)) {
		m.status = target.Name + " is hidden by the current search or favorites filter"
		return
	}
	m.viewMode = ""
	m.detailOffset = 0
	m.status = "Jumped to " + target.Name
}

func diffLines(from, to, arch string, width int) []string {
	d, err := diffKernels(from, to, arch)
	if err != nil {
		return []string{err.Error()}
	}
	lines := []string{tuiSection.Render("KERNEL RELEASE DIFF"), from + " → " + to + " · " + arch, ""}
	header := len(lines)
	for _, a := range d.Arches {
		for _, r := range a.Added {
			lines = append(lines, fmt.Sprintf("+ %s  #%d", r.Name, r.Nr))
		}
		for _, r := range a.Removed {
			lines = append(lines, fmt.Sprintf("− %s  #%d", r.Name, r.Nr))
		}
	}
	if len(lines) == header {
		lines = append(lines, "No syscall changes.")
	}
	return wrapAll(lines, width)
}
