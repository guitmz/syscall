package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFamilyInSnapshotAndCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"get", "shmget", "-a", "x64"}, "sysvipc"},
		{[]string{"get", "shmget", "-a", "x64", "-o", "json"}, `"family": "sysvipc"`},
		{[]string{"get", "read", "-a", "x64", "-o", "json"}, `"family": null`},
		{[]string{"get", "shmget", "--arch", "x64,ia32", "-o", "json"}, `"arch": "ia32"`},
	} {
		var out bytes.Buffer
		if err := run(tc.args, &out); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("%v: %s", tc.args, out.String())
		}
	}
	var out bytes.Buffer
	if err := run([]string{"search", "family:EPOLL", "-a", "x64", "-o", "json"}, &out); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name   string  `json:"name"`
		Family *string `json:"family"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("no epoll results")
	}
	for _, r := range rows {
		if r.Family == nil || *r.Family != "epoll" {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}
func TestExplorerFamilySearchAndRelated(t *testing.T) {
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "family:sysvipc"
	m.filter()
	if len(m.matches) == 0 {
		t.Fatal("no family matches")
	}
	for _, i := range m.matches {
		s := m.all[i]
		if s.Family == nil || *s.Family != "sysvipc" {
			t.Fatalf("unexpected family: %+v", s)
		}
	}
	m.input = "name:shmget"
	m.filter()
	selected := m.selected()
	if selected == nil || selected.Family == nil {
		t.Fatal("shmget missing family")
	}
	details := strings.Join(detailLines(*selected, 120), "\n")
	if !strings.Contains(details, "sysvipc") {
		t.Fatal(details)
	}
	related := m.relatedIndices()
	if len(related) == 0 {
		t.Fatal("no family relatives")
	}
	for _, i := range related {
		r := m.all[i]
		if r.Arch != selected.Arch || r.Family == nil || *r.Family != *selected.Family {
			t.Fatalf("unrelated syscall: %+v", r)
		}
	}
	if !strings.Contains(strings.Join(m.relatedLines(120), "\n"), "Family: sysvipc") {
		t.Fatal("family label missing")
	}
	// A hidden target keeps the query and only reports it.
	m.jumpRelated(1)
	if m.input != "name:shmget" || m.selected().Name != "shmget" || !strings.Contains(m.status, "hidden") {
		t.Fatalf("hidden jump changed state: %q %q", m.input, m.status)
	}
	m.input = ""
	m.filter()
	m.selectKey(favoriteKey(*selected))
	m.jumpRelated(1)
	if m.selected() == nil || m.selected().Name == "shmget" {
		t.Fatal("family jump failed")
	}
	m.input = "name:read"
	m.filter()
	if len(m.relatedIndices()) == 0 {
		t.Fatal("unclassified read should retain curated group")
	}
}
