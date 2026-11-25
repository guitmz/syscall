package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
)

//go:embed dataset/syscalls.json.gz
var embeddedDataset []byte

//go:embed dataset/provenance.json
var embeddedProvenance []byte

// syscall is one dataset row. Field order and nulls match the dataset so JSON
// output looks exactly like the source.
type syscall struct {
	Arch        string  `json:"arch"`
	Nr          int     `json:"nr"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	ABI         string  `json:"abi"`
	Entry       string  `json:"entry_point"`
	Nargs       *int    `json:"nargs"`
	Arg0        *string `json:"arg0"`
	Arg1        *string `json:"arg1"`
	Arg2        *string `json:"arg2"`
	Arg3        *string `json:"arg3"`
	Arg4        *string `json:"arg4"`
	Arg5        *string `json:"arg5"`
	Man         *string `json:"man_page"`
	Source      string  `json:"source"`
	Since       string  `json:"since"`
	Removed     *string `json:"removed_in"`
	Kconfig     *string `json:"kconfig"`
	Family      *string `json:"family"`

	since, removed kernelVersion
}

// clean strips control characters from every text field so a bad dataset
// can't send escape sequences to the terminal, and keeps nargs in range.
func (s *syscall) clean() {
	for _, f := range []*string{&s.Arch, &s.Name, &s.ABI, &s.Entry, &s.Source, &s.Since} {
		*f = cleanText(*f)
	}
	for _, f := range []*string{s.Description, s.Arg0, s.Arg1, s.Arg2, s.Arg3, s.Arg4, s.Arg5, s.Man, s.Removed, s.Kconfig, s.Family} {
		if f != nil {
			*f = cleanText(*f)
		}
	}
	if s.Nargs != nil {
		n := min(max(*s.Nargs, 0), 6)
		s.Nargs = &n
	}
}

// cleanText drops control characters and whole escape sequences, turning
// tabs and newlines into spaces.
func cleanText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != 0x1b {
			b.WriteByte(c)
			continue
		}
		// Skip a CSI (ESC [ ... final byte), an OSC (ESC ] ... BEL or ESC \)
		// or a plain two byte escape.
		switch i++; {
		case i < len(s) && s[i] == '[':
			for i++; i < len(s) && (s[i] < 0x40 || s[i] > 0x7e); i++ {
			}
		case i < len(s) && s[i] == ']':
			for i++; i < len(s) && s[i] != 0x07 && s[i] != 0x1b; i++ {
			}
			if i+1 < len(s) && s[i] == 0x1b && s[i+1] == '\\' {
				i++
			}
		}
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n':
			return ' '
		case r < 0x20 || r >= 0x7f && r < 0xa0:
			return -1
		}
		return r
	}, strings.ToValidUTF8(b.String(), ""))
}

func (s syscall) args() []*string { return []*string{s.Arg0, s.Arg1, s.Arg2, s.Arg3, s.Arg4, s.Arg5} }

// in reports whether s is in the syscall table of release v.
func (s syscall) in(v kernelVersion) bool {
	return !v.less(s.since) && (s.Removed == nil || v.less(s.removed))
}

type convention struct {
	Arch        string   `json:"arch"`
	Instruction string   `json:"instruction"`
	Nr          string   `json:"nr"`
	Return      string   `json:"return"`
	Arg0        string   `json:"arg0"`
	Arg1        string   `json:"arg1"`
	Arg2        string   `json:"arg2"`
	Arg3        string   `json:"arg3"`
	Arg4        string   `json:"arg4"`
	Arg5        string   `json:"arg5"`
	Notes       []string `json:"notes,omitempty"`
	Until       string   `json:"until,omitempty"`
}

func (c convention) registers() []string {
	return []string{c.Arg0, c.Arg1, c.Arg2, c.Arg3, c.Arg4, c.Arg5}
}

type dataset struct {
	syscalls    []syscall
	conventions map[string]convention
	kernels     map[string][]kernelVersion // scanned releases per arch, sorted
	checksum    string
}

// loadDataset is a variable so tests can swap in a small fixture.
var loadDataset = sync.OnceValues(func() (*dataset, error) {
	d, err := readDataset(embeddedDataset, embeddedProvenance)
	if err != nil {
		return nil, fmt.Errorf("embedded dataset: %w", err)
	}
	return d, nil
})

func readDataset(gz, provenance []byte) (*dataset, error) {
	z, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(z)
	if err != nil {
		return nil, err
	}
	sum, err := verifyDatasetDigest(raw, provenance)
	if err != nil {
		return nil, err
	}
	d, err := parseDataset(raw)
	if err != nil {
		return nil, err
	}
	d.checksum = sum
	return d, nil
}

func verifyDatasetDigest(raw, provenance []byte) (string, error) {
	var m struct {
		DatasetSHA256 string `json:"dataset_sha256"`
	}
	if err := json.Unmarshal(provenance, &m); err != nil {
		return "", fmt.Errorf("invalid provenance: %w", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	if m.DatasetSHA256 != digest {
		return "", errors.New("dataset checksum does not match provenance")
	}
	return digest, nil
}

func parseDataset(raw []byte) (*dataset, error) {
	var file struct {
		Kernels []struct {
			Version string `json:"version"`
			Arch    string `json:"arch"`
		} `json:"kernels"`
		Conventions []convention `json:"conventions"`
		Syscalls    []syscall    `json:"syscalls"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	d := &dataset{syscalls: file.Syscalls, conventions: map[string]convention{}, kernels: map[string][]kernelVersion{}}
	for _, k := range file.Kernels {
		v, err := parseVersion(k.Version)
		if err != nil {
			return nil, err
		}
		d.kernels[k.Arch] = append(d.kernels[k.Arch], v)
	}
	for _, versions := range d.kernels {
		slices.SortFunc(versions, compareVersions)
	}
	if len(d.kernels) == 0 || len(d.syscalls) == 0 {
		return nil, errors.New("dataset has no kernels or syscalls")
	}
	for _, c := range file.Conventions {
		for _, f := range []*string{&c.Arch, &c.Instruction, &c.Nr, &c.Return, &c.Arg0, &c.Arg1, &c.Arg2, &c.Arg3, &c.Arg4, &c.Arg5, &c.Until} {
			*f = cleanText(*f)
		}
		for i := range c.Notes {
			c.Notes[i] = cleanText(c.Notes[i])
		}
		d.conventions[c.Arch] = c
	}
	for i := range d.syscalls {
		s := &d.syscalls[i]
		s.clean()
		var err error
		if s.since, err = parseVersion(s.Since); err != nil {
			return nil, err
		}
		if s.Removed != nil {
			if s.removed, err = parseVersion(*s.Removed); err != nil {
				return nil, err
			}
		}
	}
	return d, nil
}

type kernelVersion struct{ major, minor int }

// parseVersion accepts plain "major.minor" releases. ParseUint rejects signs.
func parseVersion(s string) (kernelVersion, error) {
	major, minor, _ := strings.Cut(s, ".")
	a, errA := strconv.ParseUint(major, 10, 31)
	b, errB := strconv.ParseUint(minor, 10, 31)
	if errA != nil || errB != nil {
		return kernelVersion{}, fmt.Errorf("invalid kernel release %q", s)
	}
	return kernelVersion{int(a), int(b)}, nil
}

func (v kernelVersion) String() string { return fmt.Sprintf("%d.%d", v.major, v.minor) }

func (v kernelVersion) less(w kernelVersion) bool { return compareVersions(v, w) < 0 }

func compareVersions(v, w kernelVersion) int {
	if v.major != w.major {
		return v.major - w.major
	}
	return v.minor - w.minor
}

// releases lists the scanned releases for arch, or for every arch when it's empty.
func (d *dataset) releases(arch string) []kernelVersion {
	if arch != "" {
		return d.kernels[arch]
	}
	var all []kernelVersion
	for _, versions := range d.kernels {
		all = append(all, versions...)
	}
	slices.SortFunc(all, compareVersions)
	return slices.Compact(all)
}

func (d *dataset) knownKernel(s string) (kernelVersion, error) {
	v, err := parseVersion(s)
	if err != nil {
		return v, err
	}
	if _, found := slices.BinarySearchFunc(d.releases(""), v, compareVersions); !found {
		return v, fmt.Errorf("unknown kernel release %q", s)
	}
	return v, nil
}

func (d *dataset) covers(arch string, v kernelVersion) bool {
	return slices.Contains(d.kernels[arch], v)
}

// filter picks syscalls for arch (empty for all) and kernel. kernel can be a
// release, all, or empty for the arch's last scanned table. With neither set
// it keeps whatever hasn't been removed.
func (d *dataset) filter(arch, kernel string) (func(syscall) bool, error) {
	if arch != "" && d.kernels[arch] == nil {
		return nil, fmt.Errorf("unknown architecture %q", arch)
	}
	inArch := func(s syscall) bool { return arch == "" || s.Arch == arch }
	switch {
	case kernel == "all":
		return inArch, nil
	case kernel == "" && arch == "":
		return func(s syscall) bool { return s.Removed == nil }, nil
	case kernel == "":
		last := d.kernels[arch][len(d.kernels[arch])-1]
		return func(s syscall) bool { return inArch(s) && s.in(last) }, nil
	}
	v, err := d.knownKernel(kernel)
	if err != nil {
		return nil, err
	}
	if arch != "" && !d.covers(arch, v) {
		return nil, fmt.Errorf("%s has no syscall table for kernel %s, drop --kernel for its last scanned table or use --kernel all", arch, kernel)
	}
	return func(s syscall) bool { return inArch(s) && s.in(v) }, nil
}

func (d *dataset) collect(keep func(syscall) bool) []syscall {
	rows := []syscall{}
	for _, s := range d.syscalls {
		if keep(s) {
			rows = append(rows, s)
		}
	}
	return rows
}

var errNoMatch = errors.New("no matching syscalls")

// commonArches is what a lookup shows when no arch is given.
var commonArches = []string{"x64", "x86", "arm", "arm64", "riscv64"}

// lookup finds a syscall by name or number. arches is empty for the common
// arches, all, or a comma separated list. hidden counts rows the common
// default left out.
func lookup(query, arches, kernel string) (rows []syscall, hidden int, err error) {
	want, err := archList(arches)
	if err != nil {
		return nil, 0, err
	}
	if want == nil {
		rows, err = getSyscalls(query, "", kernel)
		return rows, 0, suggest(query, err)
	}
	common := len(want) == 0
	if common {
		want = commonArches
	}
	for _, arch := range want {
		found, err := getSyscalls(query, arch, kernel)
		// The common default skips arches that lack the release instead of failing.
		if err != nil && !common && !errors.Is(err, errNoMatch) {
			return nil, 0, err
		}
		rows = append(rows, found...)
	}
	if common {
		all, err := getSyscalls(query, "", kernel)
		if err != nil && len(rows) == 0 {
			return nil, 0, suggest(query, err)
		}
		hidden = max(0, len(all)-len(rows))
		if len(rows) == 0 && hidden > 0 {
			return nil, 0, fmt.Errorf("%s is not on %s, try --arch all", query, strings.Join(commonArches, ", "))
		}
	}
	if len(rows) == 0 {
		return nil, 0, suggest(query, fmt.Errorf("%w for %q", errNoMatch, query))
	}
	return rows, hidden, nil
}

// archList parses --arch for lookups. It returns nil for all, an empty list
// for the common default, or the named arches without blanks or repeats.
func archList(spec string) ([]string, error) {
	want := []string{}
	for _, a := range strings.Split(spec, ",") {
		if a = canonicalArch(a); a != "" && !slices.Contains(want, a) {
			want = append(want, a)
		}
	}
	if slices.Contains(want, "all") {
		if len(want) > 1 {
			return nil, errors.New("--arch all can't be mixed with other arches")
		}
		return nil, nil
	}
	return want, nil
}

// suggest adds close syscall names to a no match error.
func suggest(query string, err error) error {
	if !errors.Is(err, errNoMatch) {
		return err
	}
	d, loadErr := loadDataset()
	if loadErr != nil {
		return err
	}
	type candidate struct {
		name string
		dist int
	}
	var close []candidate
	seen := map[string]bool{}
	q := strings.ToLower(query)
	for _, s := range d.syscalls {
		if seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		dist := editDistance(q, s.Name)
		if strings.HasPrefix(s.Name, q) && len(q) >= 3 {
			dist = 1
		}
		if dist <= max(1, len(q)/3) {
			close = append(close, candidate{s.Name, dist})
		}
	}
	if len(close) == 0 {
		return err
	}
	slices.SortStableFunc(close, func(a, b candidate) int { return a.dist - b.dist })
	var names []string
	for _, c := range close[:min(3, len(close))] {
		names = append(names, c.name)
	}
	return fmt.Errorf("%w, did you mean %s", err, strings.Join(names, ", "))
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := []int{i}
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur = append(cur, min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost))
		}
		prev = cur
	}
	return prev[len(b)]
}

// getSyscalls finds a syscall by name or number.
func getSyscalls(query, arch, kernel string) ([]syscall, error) {
	d, err := loadDataset()
	if err != nil {
		return nil, err
	}
	keep, err := d.filter(arch, kernel)
	if err != nil {
		return nil, err
	}
	number, isNumber, err := syscallNumber(query)
	if err != nil {
		return nil, err
	}
	rows := d.collect(func(s syscall) bool {
		if isNumber {
			return s.Nr == number && keep(s)
		}
		return s.Name == query && keep(s)
	})
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w for %q", errNoMatch, query)
	}
	return rows, nil
}

func listSyscalls(arch, kernel string) ([]syscall, error) {
	d, err := loadDataset()
	if err != nil {
		return nil, err
	}
	keep, err := d.filter(arch, kernel)
	if err != nil {
		return nil, err
	}
	rows := d.collect(keep)
	if len(rows) == 0 {
		return nil, errNoMatch
	}
	return rows, nil
}

// Diff keys stay alphabetical so the JSON output keeps its old shape.
type kernelDiff struct {
	Arches []archDiff `json:"arches"`
	From   string     `json:"from"`
	To     string     `json:"to"`
}

type archDiff struct {
	Added   []syscall `json:"added"`
	Arch    string    `json:"arch"`
	Removed []syscall `json:"removed"`
}

// diffArches is diffKernels with lookup style --arch handling, the common
// arches by default, all, or a comma separated list.
func diffArches(fromRelease, toRelease, arches string) (kernelDiff, error) {
	want, err := archList(arches)
	if err != nil {
		return kernelDiff{}, err
	}
	if len(want) == 1 {
		return diffKernels(fromRelease, toRelease, want[0])
	}
	diff, err := diffKernels(fromRelease, toRelease, "")
	if err != nil || want == nil {
		return diff, err
	}
	for _, arch := range want {
		if !slices.ContainsFunc(diff.Arches, func(a archDiff) bool { return a.Arch == arch }) {
			return kernelDiff{}, fmt.Errorf("%s was not scanned in both releases", arch)
		}
	}
	if len(want) == 0 {
		want = commonArches
	}
	diff.Arches = slices.DeleteFunc(diff.Arches, func(a archDiff) bool { return !slices.Contains(want, a.Arch) })
	if len(diff.Arches) == 0 {
		return kernelDiff{}, fmt.Errorf("none of %s were scanned in both releases, try --arch all", strings.Join(want, ", "))
	}
	return diff, nil
}

func diffKernels(fromRelease, toRelease, arch string) (kernelDiff, error) {
	d, err := loadDataset()
	if err != nil {
		return kernelDiff{}, err
	}
	from, err := d.knownKernel(fromRelease)
	if err != nil {
		return kernelDiff{}, err
	}
	to, err := d.knownKernel(toRelease)
	if err != nil {
		return kernelDiff{}, err
	}
	result := kernelDiff{Arches: []archDiff{}, From: from.String(), To: to.String()}
	index := map[string]int{}
	for _, s := range d.syscalls {
		i, seen := index[s.Arch]
		if !seen {
			index[s.Arch] = -1
			if arch != "" && arch != s.Arch || !d.covers(s.Arch, from) || !d.covers(s.Arch, to) {
				continue
			}
			i = len(result.Arches)
			index[s.Arch] = i
			result.Arches = append(result.Arches, archDiff{Added: []syscall{}, Arch: s.Arch, Removed: []syscall{}})
		}
		if i < 0 {
			continue
		}
		a := &result.Arches[i]
		switch was, is := s.in(from), s.in(to); {
		case is && !was:
			a.Added = append(a.Added, s)
		case was && !is:
			a.Removed = append(a.Removed, s)
		}
	}
	if len(result.Arches) == 0 {
		return kernelDiff{}, errors.New("no matching architecture was scanned in both releases")
	}
	return result, nil
}

type architecture struct {
	Arch     string `json:"arch"`
	First    string `json:"first"`
	Last     string `json:"last"`
	Until    string `json:"until,omitempty"`
	Syscalls int    `json:"syscalls"`
}

func architectures() ([]architecture, error) {
	d, err := loadDataset()
	if err != nil {
		return nil, err
	}
	last := map[string]kernelVersion{}
	for arch, versions := range d.kernels {
		last[arch] = versions[len(versions)-1]
	}
	counts := map[string]int{}
	for _, s := range d.syscalls {
		if v, ok := last[s.Arch]; ok && s.in(v) {
			counts[s.Arch]++
		}
	}
	rows := []architecture{}
	for _, arch := range slices.Sorted(maps.Keys(d.kernels)) {
		versions := d.kernels[arch]
		rows = append(rows, architecture{
			Arch: arch, First: versions[0].String(), Last: versions[len(versions)-1].String(),
			Until: d.conventions[arch].Until, Syscalls: counts[arch],
		})
	}
	return rows, nil
}

// conventions returns the calling convention for arch, or every one when arch is empty.
func conventions(arch string) ([]convention, error) {
	d, err := loadDataset()
	if err != nil {
		return nil, err
	}
	if arch != "" {
		c, ok := d.conventions[arch]
		if !ok {
			return nil, fmt.Errorf("unknown architecture %q", arch)
		}
		return []convention{c}, nil
	}
	rows := []convention{}
	for _, name := range slices.Sorted(maps.Keys(d.conventions)) {
		rows = append(rows, d.conventions[name])
	}
	return rows, nil
}

func conventionFor(arch string) (convention, error) {
	d, err := loadDataset()
	if err != nil {
		return convention{}, err
	}
	c, ok := d.conventions[arch]
	if !ok {
		return c, fmt.Errorf("no calling convention for %s", arch)
	}
	return c, nil
}

// History keys stay alphabetical, like the diff.
type history struct {
	Comparison *historyComparison `json:"comparison,omitempty"`
	Releases   []historyPoint     `json:"releases"`
	Syscall    syscall            `json:"syscall"`
}

type historyComparison struct {
	From        string `json:"from"`
	PresentFrom bool   `json:"present_in_from"`
	PresentTo   bool   `json:"present_in_to"`
	To          string `json:"to"`
}

type historyPoint struct {
	Version string `json:"version"`
	Present bool   `json:"present"`
}

func syscallHistory(name, arch, from, to string) (history, error) {
	if arch == "" {
		return history{}, errors.New("history requires --arch")
	}
	if (from == "") != (to == "") {
		return history{}, errors.New("--from and --to must be provided together")
	}
	rows, err := getSyscalls(name, arch, "all")
	if err != nil {
		return history{}, err
	}
	if len(rows) != 1 {
		return history{}, errors.New("history requires a unique syscall")
	}
	d, err := loadDataset()
	if err != nil {
		return history{}, err
	}
	h := history{Releases: []historyPoint{}, Syscall: rows[0]}
	for _, v := range d.kernels[arch] {
		h.Releases = append(h.Releases, historyPoint{v.String(), h.Syscall.in(v)})
	}
	if from != "" {
		a, err := d.knownKernel(from)
		if err != nil {
			return history{}, err
		}
		b, err := d.knownKernel(to)
		if err != nil {
			return history{}, err
		}
		if !d.covers(arch, a) || !d.covers(arch, b) {
			return history{}, errors.New("architecture was not scanned in both releases")
		}
		h.Comparison = &historyComparison{From: a.String(), PresentFrom: h.Syscall.in(a), PresentTo: h.Syscall.in(b), To: b.String()}
	}
	return h, nil
}

type buildInfo struct {
	Version       string `json:"version"`
	LatestKernel  string `json:"latest_kernel"`
	DatasetSHA256 string `json:"dataset_sha256"`
	SourceURL     string `json:"source_url"`
}

func versionInfo() (buildInfo, error) {
	d, err := loadDataset()
	if err != nil {
		return buildInfo{}, err
	}
	var provenance struct {
		SourceURL string `json:"source_url"`
	}
	if err := json.Unmarshal(embeddedProvenance, &provenance); err != nil {
		return buildInfo{}, fmt.Errorf("invalid dataset provenance: %w", err)
	}
	all := d.releases("")
	return buildInfo{cliVersion(), all[len(all)-1].String(), d.checksum, provenance.SourceURL}, nil
}
