package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Build metadata, set with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "none"
)

const defaultDepth = 8

// usageError marks mistakes in how the command was invoked (exit status 2).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return &usageError{msg: fmt.Sprintf(format, a...)} }

// options holds every flag of the root command.
type options struct {
	Fn        string
	Raw       bool
	JSON      bool
	Depth     int
	Since     string
	NoNetwork bool
	Explain   bool

	// SinceTime is Since parsed; zero when unset.
	SinceTime time.Time
	// Targets are the positional arguments.
	Targets []string
}

// validate checks flag combinations and parses derived values.
func (o *options) validate() error {
	if o.Raw && o.JSON {
		return usagef("--raw and --json cannot be combined")
	}
	if o.Depth < 1 {
		return usagef("--depth must be at least 1, got %d", o.Depth)
	}
	if o.Since != "" {
		t, err := parseSince(o.Since)
		if err != nil {
			return usagef("--since: %v", err)
		}
		o.SinceTime = t
	}
	if len(o.Targets) == 0 {
		return usagef("missing target: use <file>:<line>, <file>:<start>-<end>, <file>#<Symbol> or <file> --fn <name>")
	}
	if len(o.Targets) > 1 {
		return usagef("expected one target, got %d", len(o.Targets))
	}
	return nil
}

// parseSince accepts RFC 3339 timestamps and plain YYYY-MM-DD dates.
func parseSince(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q: use YYYY-MM-DD or RFC 3339", s)
}

// analyzeFunc runs one analysis. It is a variable so tests can intercept it.
type analyzeFunc func(out io.Writer, o *options) error

func newRootCmd(stdout, stderr io.Writer, analyze analyzeFunc) *cobra.Command {
	o := &options{}
	cmd := &cobra.Command{
		Use:   "git-why <file>:<line>[-<end>] | <file>#<Symbol> | <file> --fn <name>",
		Short: "Explain why a line of code looks the way it does",
		Long: "git-why reconstructs the decision history behind a line or function: the commits that\n" +
			"shaped it, the pull requests and issues behind them, and the review discussion.\n" +
			"Every statement in the write-up cites its source.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Targets = args
			if err := o.validate(); err != nil {
				return err
			}
			return analyze(cmd.OutOrStdout(), o)
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetVersionTemplate("git-why {{.Version}} (" + commit + ")\n")

	f := cmd.Flags()
	f.StringVar(&o.Fn, "fn", "", "analyze the function, method or type with this name")
	f.BoolVar(&o.Raw, "raw", false, "print the evidence timeline only, without a model")
	f.BoolVar(&o.JSON, "json", false, "print the evidence graph as JSON")
	f.IntVar(&o.Depth, "depth", defaultDepth, "maximum number of commits to walk")
	f.StringVar(&o.Since, "since", "", "ignore history older than this date (YYYY-MM-DD)")
	f.BoolVar(&o.NoNetwork, "no-network", false, "use local git history only")
	f.BoolVar(&o.Explain, "explain", false, "show relevance scores (debugging)")
	return cmd
}

// run executes the CLI with the given arguments against the real environment.
func run(args []string) error {
	cmd := newRootCmd(os.Stdout, os.Stderr, analyze)
	cmd.SetArgs(args)
	return cmd.Execute()
}
