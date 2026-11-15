package main

import (
	"slices"
	"strings"
	"testing"
)

func TestSearchRanking(t *testing.T) {
	rows, err := searchSyscalls("read", "x64", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 3 || rows[0].Name != "read" || !strings.HasPrefix(rows[1].Name, "read") {
		t.Fatalf("exact then prefix matches expected: %v", rows[:min(3, len(rows))])
	}
	rows, err = searchSyscalls("name:open fd:true", "x64", "")
	if err != nil || len(rows) == 0 {
		t.Fatalf("%v %v", rows, err)
	}
	for _, s := range rows {
		if !strings.Contains(s.Name, "open") || !slices.ContainsFunc(s.args(), hasFDWord) {
			t.Fatalf("unexpected match %s", signature(s))
		}
	}
	if rows, _ = searchSyscalls("rdlnkat", "x64", ""); len(rows) != 1 || rows[0].Name != "readlinkat" {
		t.Fatalf("fuzzy match: %v", rows)
	}
	if _, err = searchSyscalls(" ", "", ""); err == nil {
		t.Fatal("empty query accepted")
	}
}

func TestExplorerSharesSearchRanking(t *testing.T) {
	m, err := newExplorer("x64", "")
	if err != nil {
		t.Fatal(err)
	}
	m.input = "write"
	m.filter()
	cli, err := searchSyscalls("write", "x64", "")
	if err != nil || len(cli) != len(m.matches) {
		t.Fatalf("explorer found %d, search found %d (%v)", len(m.matches), len(cli), err)
	}
	for i, index := range m.matches {
		if m.all[index].Name != cli[i].Name {
			t.Fatalf("order differs at %d: %s vs %s", i, m.all[index].Name, cli[i].Name)
		}
	}
}
