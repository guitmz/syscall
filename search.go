package main

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// rank returns the indexes of rows that match every term, best matches
// first. Ties keep dataset order. keep can drop rows before matching.
func rank(rows []syscall, query string, keep func(syscall) bool) []int {
	terms := strings.Fields(strings.ToLower(query))
	scores := make([]int, len(rows))
	var matches []int
next:
	for i, s := range rows {
		if !keep(s) {
			continue
		}
		total := 0
		for _, term := range terms {
			score := termScore(s, term)
			if score < 0 {
				continue next
			}
			total += score
		}
		scores[i] = total
		matches = append(matches, i)
	}
	slices.SortStableFunc(matches, func(a, b int) int { return scores[b] - scores[a] })
	return matches
}

// termScore rates one lowercased term against s, or returns -1 on a miss.
// Field terms like name:open or fd:true only filter, so they score 0.
func termScore(s syscall, term string) int {
	name := strings.ToLower(s.Name)
	if field, value, ok := strings.Cut(term, ":"); ok {
		switch field {
		case "name":
			return hit(strings.Contains(name, value))
		case "desc":
			return hit(strings.Contains(strings.ToLower(deref(s.Description)), value))
		case "family":
			return hit(s.Family != nil && strings.EqualFold(*s.Family, value))
		case "arg":
			return hit(slices.ContainsFunc(s.args(), func(a *string) bool {
				return a != nil && strings.Contains(strings.ToLower(*a), value)
			}))
		case "nargs":
			n, err := strconv.Atoi(value)
			return hit(err == nil && s.Nargs != nil && *s.Nargs == n)
		case "fd":
			if value == "true" {
				return hit(slices.ContainsFunc(s.args(), hasFDWord))
			}
		}
	}
	if strings.HasPrefix(term, "0x") {
		n, ok, err := syscallNumber(term)
		return hit(ok && err == nil && s.Nr == n)
	}
	switch {
	case name == term:
		return 100
	case strings.HasPrefix(name, term):
		return 80
	case strings.Contains(name, term):
		return 60
	case strings.Contains(strings.ToLower(deref(s.Description)), term):
		return 40
	case strings.Contains(strings.ToLower(s.Arch+" "+strconv.Itoa(s.Nr)+" "+deref(s.Family)), term):
		return 30
	case name != "" && term[0] == name[0] && subsequence(term, name):
		// Fuzzy matches must start like the name, so open skips copy_file_range.
		return 20
	}
	return -1
}

func hit(ok bool) int {
	if ok {
		return 0
	}
	return -1
}

// subsequence reports whether the runes of q appear in s in order.
func subsequence(q, s string) bool {
	r := []rune(q)
	if len(r) == 0 {
		return false
	}
	i := 0
	for _, c := range s {
		if c == r[i] {
			i++
			if i == len(r) {
				return true
			}
		}
	}
	return false
}

// hasFDWord reports whether an argument has a word ending in fd, like fd or dirfd.
func hasFDWord(a *string) bool {
	if a == nil {
		return false
	}
	words := strings.FieldsFunc(strings.ToLower(*a), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
	})
	return slices.ContainsFunc(words, func(w string) bool { return strings.HasSuffix(w, "fd") })
}

// checkTerms catches filter terms the matcher would quietly treat as misses.
func checkTerms(query string) error {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return errors.New("provide a query")
	}
	for _, term := range terms {
		field, value, ok := strings.Cut(term, ":")
		if !ok {
			continue
		}
		switch field {
		case "name", "desc", "family", "arg":
			if value == "" {
				return fmt.Errorf("%s needs a value, like %s:open", term, field)
			}
		case "nargs":
			if n, err := strconv.Atoi(value); err != nil || n < 0 || n > 6 {
				return fmt.Errorf("%s needs a number from 0 to 6", term)
			}
		case "fd":
			if value != "true" {
				return errors.New("fd only takes fd:true")
			}
		}
	}
	return nil
}

func searchSyscalls(query, arch, kernel string) ([]syscall, error) {
	if err := checkTerms(query); err != nil {
		return nil, err
	}
	if arch == "all" {
		arch = ""
	}
	d, err := loadDataset()
	if err != nil {
		return nil, err
	}
	keep, err := d.filter(arch, kernel)
	if err != nil {
		return nil, err
	}
	rows := []syscall{}
	for _, i := range rank(d.syscalls, query, keep) {
		rows = append(rows, d.syscalls[i])
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("nothing matches %q", query)
	}
	return rows, nil
}
