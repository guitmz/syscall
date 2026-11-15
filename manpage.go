package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type manPageResult struct{ key, text string }

type cappedManOutput struct {
	bytes.Buffer
	truncated bool
}

func (b *cappedManOutput) Write(p []byte) (int, error) {
	const limit = 1 << 20
	n := min(len(p), limit-b.Len())
	b.truncated = b.truncated || n < len(p)
	b.Buffer.Write(p[:n])
	return len(p), nil
}

func loadManPage(key, name string) tea.Cmd {
	return func() tea.Msg {
		result := manPageResult{key: key}
		if name == "" {
			result.text = "Invalid syscall name for local man lookup."
			return result
		}
		for _, r := range name {
			if r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				result.text = "Invalid syscall name for local man lookup."
				return result
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "man", "2", name)
		cmd.Env = append(os.Environ(), "MANPAGER=cat", "PAGER=cat", "TERM=dumb", "MANWIDTH=80")
		var output cappedManOutput
		cmd.Stdout = &output
		if err := cmd.Run(); err != nil {
			result.text = fmt.Sprintf("Local man page %s(2) unavailable. Install the Linux man-pages package or use m to open the online reference.", name)
			return result
		}
		var text []rune
		for _, r := range ansi.Strip(output.String()) {
			switch {
			case r == '\b':
				if len(text) > 0 && text[len(text)-1] != '\n' {
					text = text[:len(text)-1]
				}
			case r == '\n' || r == '\t' || !unicode.IsControl(r):
				text = append(text, r)
			}
		}
		result.text = "LOCAL MAN PAGE · " + name + "(2)\n\n" + strings.TrimSpace(string(text))
		if output.truncated {
			result.text += "\n\n[Man page truncated at 1 MiB.]"
		}
		return result
	}
}
