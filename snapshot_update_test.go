package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSnapshotFields(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"get", "read", "--arch", "ia32", "--output", "json"}, `"arch": "ia32"`},
		{[]string{"get", "madvise", "--arch", "x64"}, "CONFIG_ADVISE_SYSCALLS"},
		{[]string{"get", "madvise", "--arch", "x64", "--output", "json"}, `"kconfig": "CONFIG_ADVISE_SYSCALLS"`},
		{[]string{"conventions", "x64"}, "clobbers rcx"},
		{[]string{"conventions", "x64", "--output", "json"}, `"notes"`},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := run(tc.args, &out); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("%v: missing %q in %s", tc.args, tc.want, out.String())
		}
	}
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "name:madvise"
	m.filter()
	if len(m.matches) == 0 {
		t.Fatal("madvise not found")
	}
	s := m.selected()
	details := strings.Join(detailLines(*s, 100), "\n")
	if !strings.Contains(details, "CONFIG_ADVISE_SYSCALLS") || !strings.Contains(details, "clobbers rcx") {
		t.Fatal(details)
	}
	code, err := snippet("read", "ia32", "go", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(code, "CONFIG_IA32_EMULATION") || !strings.Contains(code, "linux && 386") {
		t.Fatal(code)
	}
}

func TestTraceAndCompareNotes(t *testing.T) {
	var out bytes.Buffer
	if err := annotateTrace(strings.NewReader("28\n"), &out, "x64", "", "table", traceOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "CONFIG_ADVISE_SYSCALLS") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := annotateTrace(strings.NewReader("openat(-100, \"file\", 0x80000) = 3\n"), &out, "ia32", "", "table", traceOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "O_CLOEXEC") {
		t.Fatal(out.String())
	}
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "name:read"
	m.filter()
	if len(m.matches) == 0 {
		t.Fatal("read not found")
	}
	lines := strings.Join(compareLines(*m.selected(), "", 120), "\n")
	if !strings.Contains(lines, "ia32") || !strings.Contains(lines, "clobbers rcx") {
		t.Fatal("comparison lacks new ABI or notes")
	}
}
