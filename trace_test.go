package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTraceFormatsAndSummary(t *testing.T) {
	input := `[pid 42] openat(AT_FDCWD, "a,b", 0x80200, 0666) = -1 ENOENT (No such file or directory)
123 12:34:56.123 read(3, "x,y", 5) = 2
  12.345 ( 0.020 ms): demo/42 openat(dfd: -100, filename: 0, flags: 0) = 3
read(3,  <unfinished ...>
0x0 argument
`
	var out bytes.Buffer
	err := annotateTrace(strings.NewReader(input), &out, "amd64", "", "json", traceOptions{Format: "auto", Summary: true})
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Lines   []traceLine  `json:"lines"`
		Summary []traceCount `json:"summary"`
	}
	if err = json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Lines) != 5 || v.Lines[0].Name != "openat" || v.Lines[0].Error != "ENOENT" || !strings.Contains(strings.Join(v.Lines[0].Decoded, ""), "O_CLOEXEC") || v.Lines[2].Name != "openat" || !strings.Contains(strings.Join(v.Lines[2].Decoded, ""), "O_RDONLY") || v.Lines[3].Name != "" || v.Lines[4].Number == nil {
		t.Fatalf("%s", out.String())
	}
	if len(v.Summary) != 2 || v.Summary[0].Calls != 2 || v.Summary[0].Errors != 1 {
		t.Fatalf("summary: %+v", v.Summary)
	}
	out.Reset()
	err = annotateTrace(strings.NewReader(input), &out, "x64", "", "table", traceOptions{Format: "auto", ErrorsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\n") != 1 || !strings.Contains(out.String(), "ENOENT") {
		t.Fatal(out.String())
	}
}

func TestTraceConservativeParsing(t *testing.T) {
	for _, line := range []string{"--- SIGTERM ---", "read(3, <unfinished ...>", "<... read resumed> \"x\", 1) = 1", "read(3, \"x\")", "just text"} {
		name, _, _ := traceCall(line, "auto")
		if name != "" {
			t.Fatalf("parsed incomplete line %q", line)
		}
	}
}

func TestDecodeArguments(t *testing.T) {
	for _, tc := range []struct {
		name, arch string
		args       []string
		want       string
	}{
		{"openat", "x64", []string{"1", "x", "0x280000"}, "arg2=O_RDONLY|O_CLOEXEC|0x200000"},
		{"open", "arm64", []string{"x", "flags: 0x42"}, "arg1=O_RDWR|O_CREAT"},
		{"socket", "x64", []string{"2", "0x80801", "0"}, "arg0=AF_INET arg1=SOCK_STREAM|SOCK_NONBLOCK|SOCK_CLOEXEC"},
		{"openat", "mips64", []string{"1", "x", "0x80000"}, ""},
		{"socket", "mips64", []string{"2", "1"}, ""},
	} {
		if got := strings.Join(decodeArguments(tc.name, tc.args, tc.arch), " "); got != tc.want {
			t.Errorf("%s on %s: got %q, want %q", tc.name, tc.arch, got, tc.want)
		}
	}
}

func TestArchitectureAliases(t *testing.T) {
	for k, w := range map[string]string{"amd64": "x64", "x86_64": "x64", "aarch64": "arm64", "i686": "x86", "mips64": "mips64"} {
		if canonicalArch(k) != w {
			t.Fatal(k)
		}
	}
	var out bytes.Buffer
	if err := run([]string{"get", "read", "--arch", "amd64", "--output", "json"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"arch": "x64"`) {
		t.Fatal(out.String())
	}
}

func TestTracePerfLines(t *testing.T) {
	for _, tc := range []struct{ line, name, errno string }{
		{`     0.000 ( 0.004 ms): cat/1234 openat(dfd: CWD, filename: "/etc/ld.so.cache", flags: RDONLY|CLOEXEC) = 3`, "openat", ""},
		{`     0.010: cat/1234 openat(dfd: CWD, filename: "/nope") = -1 ENOENT (No such file or directory)`, "openat", "ENOENT"},
		{`  1.234 (0.001 ms): Web-Content/99 read(fd: 3, buf: 0x7ffd, count: 8) = 8`, "read", ""},
		{`     0.000 cat/1234 syscalls:sys_enter_openat(__syscall_nr: 257, dfd: -100, filename: 0x7f00, flags: 524288, mode: 0)`, "openat", ""},
	} {
		name, _, errno := traceCall(tc.line, "perf")
		if name != tc.name || errno != tc.errno {
			t.Errorf("%q: got %q %q", tc.line, name, errno)
		}
	}
	// tracepoints drop __syscall_nr so flags still decode by position
	_, args, _ := traceCall(`0.000 cat/1 syscalls:sys_enter_openat(__syscall_nr: 257, dfd: -100, filename: 0x7f00, flags: 524288, mode: 0)`, "auto")
	if got := strings.Join(decodeArguments("openat", args, "x64"), ""); got != "arg2=O_RDONLY|O_CLOEXEC" {
		t.Fatal(got)
	}
}

func TestTraceStrictParsing(t *testing.T) {
	for line, errno := range map[string]string{
		`openat(AT_FDCWD, "x", O_RDONLY) = -1 ENOENT (No such file or directory)`: "ENOENT",
		`read(3, "= -1 EIO", 8) = 8`:   "",
		`read(3, "x", 1) = -0 EIO`:     "",
		`read(3, "x", 1) = -1 E9`:      "",
		`read(3, "x", 1) = 1 <0.0001>`: "",
	} {
		if _, _, got := traceCall(line, "auto"); got != errno {
			t.Errorf("%q: errno %q, want %q", line, got, errno)
		}
	}
	for _, line := range []string{
		`hello(world) and then x = 1`,
		`read(3, [1, 2}, 8) = 1`,
		`42 +++ killed by SIGSEGV +++`,
		`[pid 7] --- SIGCHLD {si_signo=SIGCHLD} ---`,
	} {
		if name, _, _ := traceCall(line, "auto"); name != "" {
			t.Errorf("%q parsed as %q", line, name)
		}
	}
	if got := decodeArguments("socket", []string{"2:10", "1"}, "x64"); strings.Join(got, " ") != "arg1=SOCK_STREAM" {
		t.Errorf("socket colon: %v", got)
	}
	if got := decodeArguments("open", []string{"x", "-2147483648"}, "x64"); strings.Join(got, "") != "arg1=O_RDONLY|0x80000000" {
		t.Errorf("negative flags: %v", got)
	}
}

func TestTraceNoiseAndControlChars(t *testing.T) {
	var out bytes.Buffer
	input := "42 +++ killed by SIGSEGV +++\n\x1b]52;c;evil\x07hi\x1b[31m red\x9b\x00\tok\r\n"
	if err := annotateTrace(strings.NewReader(input), &out, "x64", "", "table", traceOptions{}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "42 +++ killed by SIGSEGV +++\nhi red ok\n" {
		t.Fatalf("%q", out.String())
	}
}

func TestTraceLongLinesAndStreaming(t *testing.T) {
	input := "0\n" + strings.Repeat("x", maxTraceLine+10) + "\n1\n"
	var out bytes.Buffer
	if err := annotateTrace(strings.NewReader(input), &out, "x64", "", "json", traceOptions{}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized line error, got %v", err)
	}
	for _, summary := range []bool{false, true} {
		out.Reset()
		if err := annotateTrace(strings.NewReader(""), &out, "x64", "", "json", traceOptions{Summary: summary}); err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("%q: %v", out.String(), err)
		}
	}
}
