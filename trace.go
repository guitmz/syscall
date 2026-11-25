package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type traceLine struct {
	Input   string    `json:"input"`
	Number  *int      `json:"number,omitempty"`
	Name    string    `json:"name,omitempty"`
	Error   string    `json:"error,omitempty"`
	Decoded []string  `json:"decoded,omitempty"`
	Matches []syscall `json:"matches"`
}
type traceCount struct {
	Name   string `json:"name"`
	Calls  int    `json:"calls"`
	Errors int    `json:"errors"`
}
type traceOptions struct {
	Format, Only        string
	ErrorsOnly, Summary bool
}

var (
	straceLine = regexp.MustCompile(`^\s*(?:\[pid\s+[0-9]+\]\s*|[0-9]+\s+)?(?:[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?\s+)?([A-Za-z_][A-Za-z_0-9]*)\(`)
	// perf trace: "0.000 ( 0.004 ms): cat/1234 openat(...)", the colon and
	// duration are optional, and -e tracepoints show up as syscalls:sys_enter_x
	perfLine    = regexp.MustCompile(`^\s*(?:[0-9]+\.[0-9]+:?\s*)?(?:\(\s*[0-9.]+\s*ms\):?\s*)?(?:\S+/[0-9]+:?\s+)?(syscalls:sys_enter_)?([A-Za-z_][A-Za-z_0-9]*)\(`)
	errnoResult = regexp.MustCompile(`^\s*=\s*-[1-9][0-9]*\s+(E[A-Z][A-Z0-9_]*)\b`)
	// strace signal and exit notes, optionally after a pid or timestamp
	straceNote = regexp.MustCompile(`^\s*(?:\[pid\s+[0-9]+\]\s*|[0-9]+\s+)?(?:[0-9:.]+\s+)?(?:\+\+\+ .* \+\+\+|--- .* ---)\s*$`)
)

// traceNoise is strace output that is never a call or a number.
func traceNoise(line string) bool {
	return strings.Contains(line, "<unfinished ...>") || strings.Contains(line, "<... ") || straceNote.MatchString(line)
}

// traceCall parses a complete strace or perf line. Split or partial lines
// return an empty name so they pass through untouched.
func traceCall(line, format string) (name string, args []string, errno string) {
	if traceNoise(line) {
		return "", nil, ""
	}
	var match []int
	nameAt, tracepoint := 2, false
	if format == "auto" || format == "strace" {
		match = straceLine.FindStringSubmatchIndex(line)
	}
	if match == nil && (format == "auto" || format == "perf") {
		match = perfLine.FindStringSubmatchIndex(line)
		nameAt, tracepoint = 4, match != nil && match[2] >= 0
	}
	if match == nil {
		return "", nil, ""
	}
	args, end := scanTraceArgs(line, match[1])
	if end < 0 {
		return "", nil, ""
	}
	// tracepoints have no result, real calls must end in "= something"
	rest := line[end+1:]
	if tracepoint {
		if len(args) > 0 && strings.HasPrefix(args[0], "__syscall_nr") {
			args = args[1:]
		}
	} else if !strings.HasPrefix(strings.TrimSpace(rest), "=") {
		return "", nil, ""
	}
	if m := errnoResult.FindStringSubmatch(rest); m != nil {
		errno = m[1]
	}
	return line[match[nameAt]:match[nameAt+1]], args, errno
}

// scanTraceArgs splits the argument list starting at start on top level
// commas, skipping quoted strings and nested brackets. It returns the index of
// the closing paren, or -1 if there is none.
func scanTraceArgs(line string, start int) ([]string, int) {
	var args []string
	var open []byte // closers we still expect
	from := start
	quoted, escape := false, false
	for i := start; i < len(line); i++ {
		c := line[i]
		if quoted {
			switch {
			case escape:
				escape = false
			case c == '\\':
				escape = true
			case c == '"':
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '(':
			open = append(open, ')')
		case '[':
			open = append(open, ']')
		case '{':
			open = append(open, '}')
		case ')', ']', '}':
			switch {
			case len(open) > 0 && open[len(open)-1] == c:
				open = open[:len(open)-1]
			case len(open) == 0 && c == ')':
				if strings.TrimSpace(line[start:i]) == "" {
					return nil, i
				}
				return append(args, strings.TrimSpace(line[from:i])), i
			default:
				return nil, -1 // mismatched, don't guess
			}
		case ',':
			if len(open) == 0 {
				args = append(args, strings.TrimSpace(line[from:i]))
				from = i + 1
			}
		}
	}
	return nil, -1
}

type bitName struct {
	bit  uint64
	name string
}

var (
	socketFamilies = map[uint64]string{0: "AF_UNSPEC", 1: "AF_UNIX", 2: "AF_INET", 10: "AF_INET6", 16: "AF_NETLINK", 17: "AF_PACKET"}
	socketTypes    = map[uint64]string{1: "SOCK_STREAM", 2: "SOCK_DGRAM", 3: "SOCK_RAW", 4: "SOCK_RDM", 5: "SOCK_SEQPACKET", 6: "SOCK_DCCP"}
	socketFlags    = []bitName{{0x800, "SOCK_NONBLOCK"}, {0x80000, "SOCK_CLOEXEC"}}
	openAccess     = map[uint64]string{0: "O_RDONLY", 1: "O_WRONLY", 2: "O_RDWR"}
	openFlags      = []bitName{{0x40, "O_CREAT"}, {0x80, "O_EXCL"}, {0x100, "O_NOCTTY"}, {0x200, "O_TRUNC"}, {0x400, "O_APPEND"}, {0x800, "O_NONBLOCK"}, {0x10000, "O_DIRECTORY"}, {0x20000, "O_NOFOLLOW"}, {0x80000, "O_CLOEXEC"}}
)

// decodeFlags names the low bits under mask from base, then each known flag,
// and prints whatever is left as hex.
func decodeFlags(n, mask uint64, base map[uint64]string, flags []bitName) string {
	label, ok := base[n&mask]
	if !ok {
		label = fmt.Sprintf("0x%x", n&mask)
	}
	parts := []string{label}
	rest := n &^ mask
	for _, f := range flags {
		if rest&f.bit != 0 {
			parts = append(parts, f.name)
			rest &^= f.bit
		}
	}
	if rest != 0 {
		parts = append(parts, fmt.Sprintf("0x%x", rest))
	}
	return strings.Join(parts, "|")
}

var argLabel = regexp.MustCompile(`^\s*[A-Za-z_][A-Za-z_0-9]*\s*$`)

// decodeArguments only knows the generic Linux values, so other ABIs get nothing.
func decodeArguments(name string, args []string, arch string) []string {
	switch arch {
	case "x64", "x86", "ia32", "arm64", "riscv64", "loongarch64":
	default:
		return nil
	}
	// perf prints "name: value", negative ints are read as 32 bit flags
	parse := func(raw string, onlyPrefix string) (uint64, error) {
		raw = strings.TrimSpace(raw)
		if prefix, value, ok := strings.Cut(raw, ":"); ok && argLabel.MatchString(prefix) && (onlyPrefix == "" || strings.TrimSpace(prefix) == onlyPrefix) {
			raw = strings.TrimSpace(value)
		}
		if n, err := strconv.ParseInt(raw, 0, 32); err == nil && n < 0 {
			return uint64(uint32(n)), nil
		}
		return strconv.ParseUint(raw, 0, 64)
	}
	if name == "socket" && len(args) >= 2 {
		var decoded []string
		if domain, err := parse(args[0], ""); err == nil {
			if label, ok := socketFamilies[domain]; ok {
				decoded = append(decoded, "arg0="+label)
			}
		}
		if typ, err := parse(args[1], ""); err == nil {
			decoded = append(decoded, "arg1="+decodeFlags(typ, 15, socketTypes, socketFlags))
		}
		return decoded
	}
	index := map[string]int{"open": 1, "openat": 2}[name]
	if index == 0 || len(args) <= index {
		return nil
	}
	n, err := parse(args[index], "flags")
	if err != nil {
		return nil
	}
	return []string{fmt.Sprintf("arg%d=%s", index, decodeFlags(n, 3, openAccess, openFlags))}
}

func annotateTrace(in io.Reader, out io.Writer, arch, kernel, output string, o traceOptions) error {
	if arch == "" {
		return errors.New("trace requires --arch")
	}
	arch = canonicalArch(arch)
	if o.Format == "" {
		o.Format = "auto"
	}
	if o.Format != "auto" && o.Format != "numbers" && o.Format != "strace" && o.Format != "perf" {
		return errors.New("trace format must be auto, numbers, strace, or perf")
	}
	d, err := loadDataset()
	if err != nil {
		return err
	}
	keep, err := d.filter(arch, kernel)
	if err != nil {
		return err
	}
	byNumber := map[int][]syscall{}
	byName := map[string][]syscall{}
	for _, s := range d.syscalls {
		if keep(s) {
			byNumber[s.Nr] = append(byNumber[s.Nr], s)
			byName[s.Name] = append(byName[s.Name], s)
		}
	}
	counts := map[string]*traceCount{}
	js := &jsonStream{out: out, summary: o.Summary}
	r := bufio.NewReader(in)
	for {
		line, long, err := readTraceLine(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if long {
			return fmt.Errorf("trace line exceeds %d bytes", maxTraceLine)
		}
		t := traceLine{Input: line, Matches: []syscall{}}
		if !traceNoise(line) {
			if o.Format != "numbers" {
				if name, args, errno := traceCall(line, o.Format); name != "" {
					t.Name, t.Error = name, errno
					t.Matches = append(t.Matches, byName[name]...)
					t.Decoded = decodeArguments(name, args, arch)
				}
			}
			fields := strings.Fields(line)
			if t.Name == "" && len(fields) > 0 && (o.Format == "numbers" || o.Format == "auto") {
				if v, ok, e := syscallNumber(fields[0]); e == nil && ok {
					t.Number = &v
					t.Matches = append(t.Matches, byNumber[v]...)
				}
			}
			// ambiguous numbers keep an empty name so they stay out of --only and the summary
			if t.Name == "" && len(t.Matches) == 1 {
				t.Name = t.Matches[0].Name
			}
		}
		if o.Only != "" && t.Name != o.Only || o.ErrorsOnly && t.Error == "" {
			continue
		}
		if o.Summary && t.Name != "" {
			if counts[t.Name] == nil {
				counts[t.Name] = &traceCount{Name: t.Name}
			}
			counts[t.Name].Calls++
			if t.Error != "" {
				counts[t.Name].Errors++
			}
		}
		if output == "json" {
			if err = js.line(t); err != nil {
				return err
			}
			continue
		}
		parts := []string{}
		for _, s := range t.Matches {
			text := signature(s)
			if s.Kconfig != nil && *s.Kconfig != "" {
				text += " [" + *s.Kconfig + "]"
			}
			parts = append(parts, text)
		}
		parts = append(parts, t.Decoded...)
		if len(parts) > 0 {
			line += "  # " + strings.Join(parts, "; ")
		}
		if _, err = fmt.Fprintln(out, cleanText(line)); err != nil {
			return err
		}
	}
	summary := []traceCount{}
	for _, c := range counts {
		summary = append(summary, *c)
	}
	sort.Slice(summary, func(i, j int) bool {
		if summary[i].Calls != summary[j].Calls {
			return summary[i].Calls > summary[j].Calls
		}
		return summary[i].Name < summary[j].Name
	})
	if output == "json" {
		return js.close(summary)
	}
	if !o.Summary {
		return nil
	}
	if _, err = fmt.Fprintln(out, "\nSYSCALL  CALLS  ERRORS"); err != nil {
		return err
	}
	for _, s := range summary {
		if _, err = fmt.Fprintf(out, "%s  %d  %d\n", s.Name, s.Calls, s.Errors); err != nil {
			return err
		}
	}
	return nil
}

const maxTraceLine = 1 << 20

// readTraceLine returns the next line without its line ending. Anything past
// maxTraceLine is dropped and long is set.
func readTraceLine(r *bufio.Reader) (line string, long bool, err error) {
	var buf []byte
	got := false
	for {
		chunk, err := r.ReadSlice('\n')
		got = got || len(chunk) > 0
		chunk = bytes.TrimSuffix(chunk, []byte("\n"))
		if room := maxTraceLine - len(buf); len(chunk) > room {
			chunk, long = chunk[:room], true
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF && got {
			err = nil
		}
		return string(bytes.TrimSuffix(buf, []byte("\r"))), long, err
	}
}

// jsonStream writes trace lines as they come, as a bare array or as
// {"lines": [...], "summary": [...]} when a summary was asked for.
type jsonStream struct {
	out     io.Writer
	summary bool
	n       int
}

func (j *jsonStream) indent() string {
	if j.summary {
		return "    "
	}
	return "  "
}

func (j *jsonStream) line(t traceLine) error {
	head := ",\n"
	if j.n == 0 {
		head = "[\n"
		if j.summary {
			head = "{\n  \"lines\": [\n"
		}
	}
	j.n++
	b, err := indentJSON(t, j.indent())
	if err == nil {
		_, err = fmt.Fprintf(j.out, "%s%s%s", head, j.indent(), b)
	}
	return err
}

func (j *jsonStream) close(summary []traceCount) error {
	end := "\n" + j.indent()[2:] + "]"
	if j.n == 0 {
		end = "[]"
		if j.summary {
			end = "{\n  \"lines\": []"
		}
	}
	if !j.summary {
		_, err := fmt.Fprintln(j.out, end)
		return err
	}
	b, err := indentJSON(summary, "  ")
	if err == nil {
		_, err = fmt.Fprintf(j.out, "%s,\n  \"summary\": %s\n}\n", end, b)
	}
	return err
}

func indentJSON(v any, prefix string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent(prefix, "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
