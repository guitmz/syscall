package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func favoriteKey(s syscall) string { return fmt.Sprintf("%s:%d:%s", s.Arch, s.Nr, s.Name) }

func favoritesPath() (string, error) {
	p, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "syscall", "favorites.json"), nil
}

func readFavorites() (map[string]bool, error) {
	p, err := favoritesPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []string
	if err = json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("reading favorites: %w", err)
	}
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return m, nil
}

func saveFavorites(m map[string]bool) error {
	p, err := favoritesPath()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".favorites-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func openReference(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return errors.New("no valid link available")
	}
	cmd := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		return errors.New("opening links is not supported on Windows")
	}
	// Check up front so a missing opener is reported instead of a fake success.
	path, err := exec.LookPath(cmd)
	if err != nil {
		return fmt.Errorf("%s not found", cmd)
	}
	c := exec.Command(path, u.String())
	if err := c.Start(); err != nil {
		return err
	}
	go func() { _ = c.Wait() }()
	return nil
}

func copyText(text string) error {
	names := [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	if runtime.GOOS == "darwin" {
		names = [][]string{{"pbcopy"}}
	}
	var failures []error
	for _, args := range names {
		if _, err := exec.LookPath(args[0]); err != nil {
			continue
		}
		// A hung clipboard helper must not freeze the TUI.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		c := exec.CommandContext(ctx, args[0], args[1:]...)
		c.Stdin = strings.NewReader(text)
		err := c.Run()
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", args[0], err))
			continue
		}
		return nil
	}
	if len(failures) > 0 {
		return fmt.Errorf("clipboard commands failed: %w", errors.Join(failures...))
	}
	return errors.New("no clipboard tool found (install wl-copy, xclip, or xsel)")
}
