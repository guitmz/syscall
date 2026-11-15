package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestOutputAndFlags(t *testing.T) {
	cases := [][]string{{"get", "read", "--arch", "x64"}, {"arches"}, {"conventions", "x64"}, {"diff", "6.1", "6.6", "--arch", "x64"}, {"version"}}
	for _, args := range cases {
		var out bytes.Buffer
		args = append(args, "--output", "json", "--color", "always")
		if err := run(args, &out); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !json.Valid(out.Bytes()) || bytes.Contains(out.Bytes(), []byte("\x1b")) {
			t.Fatalf("invalid JSON output: %q", out.String())
		}
	}
	var plain, colored bytes.Buffer
	if err := run([]string{"get", "read", "--arch", "x64"}, &plain); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plain.Bytes(), []byte("\x1b")) {
		t.Fatal("color in piped output")
	}
	if err := run([]string{"get", "read", "--arch", "x64", "--color", "always"}, &colored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(colored.Bytes(), []byte("\x1b[")) {
		t.Fatal("missing forced color")
	}
	for _, args := range [][]string{{"get", "read", "--kernel", "6.1", "--bogus"}, {"kernels"}, {"arches", "--arch", "x64"}, {"search", "nargs:9"}, {"search", "zzzzqq"}, {"get", "read", "--arch", "all,x64"}, {"get", "read", "--color", "bad"}} {
		var out bytes.Buffer
		if err := run(args, &out); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}
func TestNarrowTableWraps(t *testing.T) {
	for _, tc := range []struct {
		width             int
		name, value, want string
	}{
		{32, "example", "A long description that must remain readable at narrow widths", "readable"},
		{18, "α", "漢字漢字漢字漢字漢字", "漢字"},
	} {
		var out bytes.Buffer
		d := display{width: tc.width, color: true}
		if err := d.table(&out, []string{"NAME", "DESCRIPTION"}, [][]string{{tc.name, tc.value}}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("missing %q in %s", tc.want, out.String())
		}
		for _, line := range strings.Split(out.String(), "\n") {
			if lipgloss.Width(line) > tc.width {
				t.Fatalf("line too wide: %q", line)
			}
		}
	}
}

func TestRedirectedTableKeepsRowsOnOneLine(t *testing.T) {
	var out bytes.Buffer
	d := newDisplay(&out, "auto")
	description := strings.Repeat("long description ", 12)
	if err := d.table(&out, []string{"NAME", "DESCRIPTION"}, [][]string{{"read", description}, {"write", description}}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "read ") || !strings.HasPrefix(lines[2], "write ") {
		t.Fatalf("expected a header and one line per row: %q", out.String())
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatal("redirected output contains color escapes")
	}
}

func TestJSONMatchesDatasetRows(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"get", "read", "-a", "x64", "-o", "json"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"arg3": null`) || !strings.Contains(out.String(), `"removed_in": null`) {
		t.Fatalf("nulls dropped: %s", out.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got) != 1 {
		t.Fatalf("%v: %s", err, out.String())
	}
	z, err := gzip.NewReader(bytes.NewReader(embeddedDataset))
	if err != nil {
		t.Fatal(err)
	}
	var file struct{ Syscalls []map[string]any }
	if err := json.NewDecoder(z).Decode(&file); err != nil {
		t.Fatal(err)
	}
	for _, row := range file.Syscalls {
		if row["arch"] == "x64" && row["name"] == "read" {
			if !reflect.DeepEqual(row, got[0]) {
				t.Fatalf("got %v, want %v", got[0], row)
			}
			return
		}
	}
	t.Fatal("read missing from dataset")
}
