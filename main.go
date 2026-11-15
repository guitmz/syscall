package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var version = "dev"

const hostArchUsage = "Architecture (default host arch, x64 off Linux)"

// cliVersion prefers the release ldflag, then the module version go install records.
func cliVersion() string {
	if info, ok := debug.ReadBuildInfo(); version == "dev" && ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func completeNames(_ *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
	d, err := loadDataset()
	if err != nil || len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var names []string
	for _, s := range d.syscalls {
		if strings.HasPrefix(s.Name, prefix) && !seen[s.Name] {
			seen[s.Name] = true
			names = append(names, s.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completeArches(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	d, err := loadDataset()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return append(slices.Sorted(maps.Keys(d.kernels)), "all"), cobra.ShellCompDirectiveNoFileComp
}

type options struct {
	arch, kernel, output, color string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "syscall:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	root := newRoot(out)
	root.SetArgs(args)
	return root.Execute()
}

func newRoot(out io.Writer) *cobra.Command {
	var o options
	root := &cobra.Command{Use: "syscall [name|number]", Short: "Explore Linux system calls", Long: "Explore Linux system calls from an embedded dataset. Pass a name or number to look it up, same as syscall get.", SilenceErrors: true, SilenceUsage: true, Args: cobra.MaximumNArgs(1)}
	root.SetOut(out)
	root.SetErr(io.Discard)
	f := root.PersistentFlags()
	f.StringVarP(&o.output, "output", "o", "table", "Output format, table or json")
	f.StringVar(&o.color, "color", "auto", "Color mode, auto, always or never")
	root.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		o.arch = canonicalArch(o.arch)
		if o.output != "table" && o.output != "json" {
			return errors.New("output must be table or json")
		}
		if o.color != "auto" && o.color != "always" && o.color != "never" {
			return errors.New("color must be auto, always, or never")
		}
		return nil
	}
	archFlag := func(c *cobra.Command, usage string) {
		c.Flags().StringVarP(&o.arch, "arch", "a", "", usage)
		_ = c.RegisterFlagCompletionFunc("arch", completeArches)
	}
	// oneArch fills in the host arch for commands that work on a single arch.
	oneArch := func(c *cobra.Command, _ []string) { o.arch = cmp.Or(o.arch, hostArch()) }
	kernelFlag := func(c *cobra.Command, usage string) { c.Flags().StringVarP(&o.kernel, "kernel", "k", "", usage) }
	// emit prints v as JSON, or calls table for everything else.
	emit := func(v any, table func(display) error) error {
		if o.output == "json" {
			return writeJSON(out, v)
		}
		return table(newDisplay(out, o.color))
	}
	syscalls := func(rows []syscall, err error) error {
		if err != nil {
			return err
		}
		return emit(rows, func(d display) error { return d.syscalls(out, rows) })
	}

	get := func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		rows, hidden, err := lookup(args[0], o.arch, o.kernel)
		if err != nil {
			return err
		}
		return emit(rows, func(d display) error {
			if len(rows) == 1 && hidden == 0 && o.arch != "" && o.arch != "all" {
				return d.details(out, rows[0])
			}
			return d.card(out, rows, hidden)
		})
	}
	getArches := "Arches to show, comma separated or all (default common ones)"
	getKernel := "Kernel release, or all to include removed syscalls"
	root.RunE = get
	root.ValidArgsFunction = completeNames
	archFlag(root, getArches)
	kernelFlag(root, getKernel)
	getCmd := &cobra.Command{Use: "get <name|number>", Short: "Look up a syscall", Example: "  syscall get read\n  syscall get read --arch x64,arm64\n  syscall get 60 --arch all --output json", Args: cobra.ExactArgs(1), RunE: get, ValidArgsFunction: completeNames}
	archFlag(getCmd, getArches)
	kernelFlag(getCmd, getKernel)

	listCmd := &cobra.Command{Use: "list", Short: "List syscalls for an architecture", Example: "  syscall list --arch arm64 --kernel 6.1", Args: cobra.NoArgs, PreRun: oneArch, RunE: func(c *cobra.Command, _ []string) error {
		return syscalls(listSyscalls(o.arch, o.kernel))
	}}
	archFlag(listCmd, hostArchUsage)
	kernelFlag(listCmd, "Kernel release, or all to include removed syscalls")

	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search syscalls by text, signature, or family",
		Long: `Search syscalls by name or description. Every term has to match. Plain terms
rank exact names first, then prefixes, then descriptions and fuzzy matches.
Terms like these filter a single field

  name:open  desc:socket  family:epoll  arg:fd  nargs:3  fd:true  0x101`,
		Example: "  syscall search open\n  syscall search 'name:open fd:true' --arch arm64",
		Args:    cobra.ExactArgs(1),
		PreRun:  oneArch,
		RunE: func(c *cobra.Command, args []string) error {
			return syscalls(searchSyscalls(args[0], o.arch, o.kernel))
		},
	}
	archFlag(searchCmd, hostArchUsage+", or all")
	kernelFlag(searchCmd, "Kernel release, or all")

	diffCmd := &cobra.Command{Use: "diff <from> <to>", Short: "Compare kernel releases", Example: "  syscall diff 6.1 6.6 --arch x64", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, args []string) error {
		diff, err := diffArches(args[0], args[1], o.arch)
		if err != nil {
			return err
		}
		return emit(diff, func(d display) error { return d.diff(out, diff) })
	}}
	archFlag(diffCmd, "Filter by architecture")

	archesCmd := &cobra.Command{Use: "arches", Short: "List architecture coverage", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		rows, err := architectures()
		if err != nil {
			return err
		}
		return emit(rows, func(d display) error { return d.architectures(out, rows) })
	}}

	conventionsCmd := &cobra.Command{Use: "conventions [arch]", Short: "Show syscall calling conventions", Example: "  syscall conventions arm64", Args: cobra.MaximumNArgs(1), RunE: func(c *cobra.Command, args []string) error {
		arch := ""
		if len(args) > 0 {
			arch = canonicalArch(args[0])
		}
		rows, err := conventions(arch)
		if err != nil {
			return err
		}
		var v any = rows
		if arch != "" {
			v = rows[0]
		}
		return emit(v, func(d display) error { return d.conventions(out, rows) })
	}}
	root.AddGroup(
		&cobra.Group{ID: "lookup", Title: "Look up:"},
		&cobra.Group{ID: "compare", Title: "Compare:"},
		&cobra.Group{ID: "tools", Title: "Tools:"},
	)
	root.AddCommand(getCmd, listCmd, searchCmd, diffCmd, archesCmd, conventionsCmd)

	var language, snippetTheme string
	snippetCmd := &cobra.Command{Use: "snippet <name|number>", Short: "Generate a syscall example", Args: cobra.ExactArgs(1), PreRun: oneArch, ValidArgsFunction: completeNames, RunE: func(c *cobra.Command, args []string) error {
		color := newDisplay(out, o.color).color
		theme, err := resolveTheme(snippetTheme, !color)
		if err != nil {
			return err
		}
		text, err := snippet(args[0], o.arch, language, o.kernel)
		if err != nil {
			return err
		}
		if o.output == "json" {
			return writeJSON(out, map[string]string{"arch": o.arch, "language": language, "code": text})
		}
		if language == "asm" {
			text = highlightAssembly(text, color, theme)
		}
		_, err = fmt.Fprint(out, text)
		return err
	}}
	archFlag(snippetCmd, hostArchUsage)
	kernelFlag(snippetCmd, "Kernel release, or all")
	snippetCmd.Flags().StringVarP(&language, "language", "l", "c", "Language, c, go or asm")
	snippetCmd.Flags().StringVar(&snippetTheme, "theme", "auto", "Assembly theme, auto, dark, light or mono")
	root.AddCommand(snippetCmd)

	var trace traceOptions
	traceCmd := &cobra.Command{Use: "trace", Short: "Annotate syscall traces from standard input", Long: "Annotate plain numeric lines or complete strace and perf trace calls. Lines we can't parse pass through unchanged.", Args: cobra.NoArgs, PreRun: oneArch, RunE: func(c *cobra.Command, _ []string) error {
		return annotateTrace(c.InOrStdin(), out, o.arch, o.kernel, o.output, trace)
	}}
	traceCmd.Flags().StringVar(&trace.Format, "format", "auto", "Input format, auto, numbers, strace or perf")
	traceCmd.Flags().StringVar(&trace.Only, "only", "", "Include only this syscall name")
	traceCmd.Flags().BoolVar(&trace.ErrorsOnly, "errors", false, "Include only lines with a recognized errno")
	traceCmd.Flags().BoolVar(&trace.Summary, "summary", false, "Include counts and recognized error counts")
	archFlag(traceCmd, hostArchUsage)
	kernelFlag(traceCmd, "Kernel release, or all")
	root.AddCommand(traceCmd)

	var from, to string
	historyCmd := &cobra.Command{Use: "history <name|number>", Short: "Show a syscall's history across scanned releases", Args: cobra.ExactArgs(1), PreRun: oneArch, ValidArgsFunction: completeNames, RunE: func(c *cobra.Command, args []string) error {
		h, err := syscallHistory(args[0], o.arch, from, to)
		if err != nil {
			return err
		}
		return emit(h, func(d display) error { return d.history(out, h) })
	}}
	archFlag(historyCmd, hostArchUsage)
	historyCmd.Flags().StringVar(&from, "from", "", "First release")
	historyCmd.Flags().StringVar(&to, "to", "", "Second release")
	root.AddCommand(historyCmd)

	var start explorerStart
	explorerCmd := &cobra.Command{Use: "explore", Short: "Browse and search syscalls interactively", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		start.arch, start.kernel = o.arch, o.kernel
		return runExplorer(out, start)
	}}
	archFlag(explorerCmd, "Filter by architecture")
	kernelFlag(explorerCmd, "Kernel release, or all to include removed syscalls")
	ef := explorerCmd.Flags()
	ef.StringVar(&start.theme, "theme", "auto", "Explorer theme, auto, dark, light or mono")
	// The rest restore a session copied with C, so they stay out of --help.
	ef.StringVar(&start.query, "query", "", "Initial search query")
	ef.StringVar(&start.selected, "select", "", "Initial selection (arch:number:name)")
	ef.StringVar(&start.view, "view", "details", "Initial view, details, compare, timeline, related, diff or asm")
	ef.StringVar(&start.diffFrom, "diff-from", "", "First release for the initial diff view")
	ef.StringVar(&start.diffTo, "diff-to", "", "Second release for the initial diff view")
	ef.StringVar(&start.diffArch, "diff-arch", "", "Architecture for the initial diff view")
	ef.BoolVar(&start.details, "open-details", false, "Open details in compact layout")
	ef.BoolVar(&start.favorites, "favorites", false, "Show favorites only")
	for _, name := range []string{"query", "select", "view", "diff-from", "diff-to", "diff-arch", "open-details", "favorites"} {
		_ = ef.MarkHidden(name)
	}
	root.AddCommand(explorerCmd)
	for group, cmds := range map[string][]*cobra.Command{
		"lookup":  {getCmd, searchCmd, listCmd, historyCmd, explorerCmd},
		"compare": {diffCmd, archesCmd, conventionsCmd},
		"tools":   {snippetCmd, traceCmd},
	} {
		for _, c := range cmds {
			c.GroupID = group
		}
	}

	root.AddCommand(&cobra.Command{Use: "version", Short: "Show the CLI and dataset version", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		info, err := versionInfo()
		if err != nil {
			return err
		}
		return emit(info, func(display) error {
			_, err := fmt.Fprintf(out, "syscall %s\ndataset Linux %s, sha256 %s\nsource %s\n", info.Version, info.LatestKernel, info.DatasetSHA256, info.SourceURL)
			return err
		})
	}})
	return root
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
