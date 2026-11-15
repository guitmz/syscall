package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHistoricalArchitectureDefaults(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"get", "read", "--arch", "ia64"}, `"arch": "ia64"`},
		{[]string{"list", "--arch", "ia64"}, `"arch": "ia64"`},
		{[]string{"search", "read", "--arch", "ia64"}, `"arch": "ia64"`},
		{[]string{"get", "--arch", "tilepro", "--", "-1"}, `"nr": -1`},
	} {
		var out bytes.Buffer
		if err := run(append([]string{"-o", "json"}, tc.args...), &out); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("%v: missing %s", tc.args, tc.want)
		}
	}
	var global bytes.Buffer
	if err := run([]string{"get", "read", "-o", "json"}, &global); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(global.String(), `"arch": "ia64"`) {
		t.Fatal("global latest lookup included removed architecture")
	}
	if err := run([]string{"get", "read", "-a", "ia64", "-k", "7.3"}, &global); err == nil || !strings.Contains(err.Error(), "no syscall table") {
		t.Fatalf("missing architecture coverage error: %v", err)
	}
	m, err := newExplorer("ia64", "")
	if err != nil || len(m.matches) == 0 {
		t.Fatalf("historical explorer: %v", err)
	}
	var trace bytes.Buffer
	if err := annotateTrace(strings.NewReader("1026\n"), &trace, "ia64", "", "table", traceOptions{}); err != nil || !strings.Contains(trace.String(), "read(") {
		t.Fatalf("historical trace: %s (%v)", trace.String(), err)
	}
}

func TestArchitectureCatalog(t *testing.T) {
	arches, err := architectures()
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range arches {
		if arch.Arch == "ia64" {
			if arch.First != "5.0" || arch.Last != "6.6" || arch.Until != "6.7" || arch.Syscalls == 0 {
				t.Fatalf("unexpected ia64 coverage: %+v", arch)
			}
			return
		}
	}
	t.Fatal("ia64 missing from architecture catalog")
}
