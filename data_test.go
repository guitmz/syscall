package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// useFixture swaps the embedded dataset for a tiny one until the test ends.
func useFixture(t *testing.T) {
	t.Helper()
	fixture := `{"kernels":[{"version":"6.1","arch":"x64"},{"version":"6.2","arch":"x64"},{"version":"6.2","arch":"arm64"}],"conventions":[{"arch":"x64","instruction":"syscall","nr":"rax","return":"rax"}],"syscalls":[{"arch":"x64","nr":0,"name":"old","abi":"64","entry_point":"sys_old","nargs":1,"arg0":"int fd","since":"6.1","removed_in":"6.2"},{"arch":"x64","nr":1,"name":"new","abi":"64","entry_point":"sys_new","nargs":0,"since":"6.2","removed_in":null}]}`
	d, err := parseDataset([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	original := loadDataset
	loadDataset = func() (*dataset, error) { return d, nil }
	t.Cleanup(func() { loadDataset = original })
}

func TestLocalQueries(t *testing.T) {
	useFixture(t)
	cases := []struct {
		args         []string
		want, absent string
	}{
		{[]string{"list", "--arch", "x64"}, "new", "old"},
		{[]string{"list", "--arch", "x64", "--kernel", "6.1"}, "old", "new"},
		{[]string{"get", "old", "--kernel", "all", "--output", "json"}, `"arg0": "int fd"`, ""},
		{[]string{"diff", "6.1", "6.2", "--arch", "x64"}, "added", ""},
		{[]string{"diff", "6.2", "6.1", "--arch", "x64"}, "removed", ""},
		{[]string{"conventions", "x64"}, "rax", ""},
		{[]string{"arches"}, "6.2", ""},
		{[]string{"version"}, "dataset Linux", ""},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := run(tc.args, &out); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) || (tc.absent != "" && strings.Contains(out.String(), tc.absent)) {
			t.Errorf("%v: %s", tc.args, out.String())
		}
	}
	var out bytes.Buffer
	if err := run([]string{"diff", "6.1", "6.2", "--arch", "arm64"}, &out); err == nil {
		t.Error("expected coverage error")
	}
	if err := run([]string{"get", "new", "--kernel", "7.0"}, &out); err == nil {
		t.Error("expected unknown kernel error")
	}
}

func TestDataStatus(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"version", "--output", "json"}, &out); err != nil {
		t.Fatal(err)
	}
	var status map[string]string
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status["dataset_sha256"]) != 64 || status["latest_kernel"] == "" {
		t.Fatal(status)
	}
}

func TestDatasetProvenanceDigest(t *testing.T) {
	raw := []byte(`{"example":true}`)
	sum := sha256.Sum256(raw)
	metadata := []byte(fmt.Sprintf(`{"dataset_sha256":"%x"}`, sum))
	if _, err := verifyDatasetDigest(raw, metadata); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyDatasetDigest([]byte("modified"), metadata); err == nil {
		t.Fatal("accepted a mismatched dataset")
	}
	if _, err := verifyDatasetDigest(raw, []byte(`{`)); err == nil {
		t.Fatal("accepted invalid provenance")
	}
}

func TestEmbeddedSnapshotIntegrity(t *testing.T) {
	d, err := loadDataset()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.kernels) == 0 || len(d.conventions) == 0 || len(d.syscalls) == 0 {
		t.Fatal("snapshot is missing data")
	}
	seen := map[string]bool{}
	for _, s := range d.syscalls {
		key := fmt.Sprintf("%s/%d/%s", s.Arch, s.Nr, s.Name)
		if s.Arch == "" || s.Name == "" || seen[key] {
			t.Fatalf("invalid or duplicate record: %s", key)
		}
		seen[key] = true
		if !d.covers(s.Arch, s.since) {
			t.Fatalf("invalid since for %s: %s", key, s.Since)
		}
		if s.Removed != nil {
			if _, err := d.knownKernel(*s.Removed); err != nil || !s.since.less(s.removed) {
				t.Fatalf("invalid removal for %s", key)
			}
		}
		if s.Nargs != nil && (*s.Nargs < 0 || *s.Nargs > 6) {
			t.Fatalf("invalid argument count for %s", key)
		}
	}
}

func TestCleanTextAndNargsClamp(t *testing.T) {
	if got := cleanText("a\x1b]8;;evil\x07b\tc\u009bd\x1b[1;31me\x1b]52;c;x\x1b\\f\x1b"); got != "ab cdef" {
		t.Fatalf("%q", got)
	}
	n := 7
	desc := "x\x1b[31m"
	s := syscall{Nargs: &n, Description: &desc}
	s.clean()
	if *s.Nargs != 6 || *s.Description != "x" {
		t.Fatalf("%d %q", *s.Nargs, *s.Description)
	}
}

func TestArchList(t *testing.T) {
	for spec, want := range map[string][]string{"": {}, ",": {}, "x64,": {"x64"}, " AMD64 ,arm64,x64": {"x64", "arm64"}, "all": nil} {
		got, err := archList(spec)
		if err != nil || !slices.Equal(got, want) || (got == nil) != (want == nil) {
			t.Errorf("%q: %v %v", spec, got, err)
		}
	}
	if _, err := archList("all,x64"); err == nil {
		t.Error("all mixed with x64 accepted")
	}
}

func TestSuggestAndHistoryRanges(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"memfd_creat"}, &out)
	if err == nil || !strings.Contains(err.Error(), "did you mean memfd_create") {
		t.Fatalf("%v", err)
	}
	if err := run([]string{"history", "read", "-a", "x64", "--color", "never"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "3.3 to ") || strings.Count(out.String(), "present") != 1 {
		t.Fatal(out.String())
	}
}
