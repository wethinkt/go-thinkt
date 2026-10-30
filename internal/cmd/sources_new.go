package cmd

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// A config written by setup lists the sources it found then. A source
// installed later has no entry: it stays off, because setup asks before
// reading each source, but it is shown as new and announced once.

// isNewSource reports whether name has no entry in an explicit sources map.
func isNewSource(cfg config.Config, name string) bool {
	if cfg.Sources == nil {
		return false
	}
	_, ok := cfg.Sources[name]
	return !ok
}

// newSources returns the available sources the config has never mentioned.
func newSources(cfg config.Config, factories []thinkt.StoreFactory) []string {
	var out []string
	for _, f := range factories {
		name := string(f.Source())
		if !isNewSource(cfg, name) {
			continue
		}
		if ok, err := f.IsAvailable(); err == nil && ok {
			out = append(out, name)
		}
	}
	return out
}

// announceNewSources writes one line per new source not yet announced and
// records it in cfg.NotifiedSources. It reports whether cfg changed; the
// caller saves it.
func announceNewSources(w io.Writer, cfg *config.Config, factories []thinkt.StoreFactory) bool {
	changed := false
	for _, name := range newSources(*cfg, factories) {
		if slices.Contains(cfg.NotifiedSources, name) {
			continue
		}
		fmt.Fprintln(w, thinktI18n.Tf("cmd.sources.newNotice",
			"thinkt: found a new source, %[1]s (off). Enable it with: thinkt config sources enable %[1]s", name))
		cfg.NotifiedSources = append(cfg.NotifiedSources, name)
		changed = true
	}
	return changed
}

// sourceEnabledLabel is the ENABLED column of `config sources list`.
func sourceEnabledLabel(cfg config.Config, name string) string {
	switch {
	case isNewSource(cfg, name):
		return thinktI18n.T("cmd.sources.new", "new")
	case isSourceDisabled(cfg, name):
		return thinktI18n.T("common.no", "no")
	default:
		return thinktI18n.T("common.yes", "yes")
	}
}

// writeNewSourcesHint follows the sources table with the enable command for
// each available source the config has never mentioned.
func writeNewSourcesHint(w io.Writer, cfg config.Config, srcs []thinkt.SourceInfo) {
	first := true
	for _, s := range srcs {
		if !s.Available || !isNewSource(cfg, string(s.Source)) {
			continue
		}
		if first {
			fmt.Fprintln(w)
			first = false
		}
		fmt.Fprintln(w, thinktI18n.Tf("cmd.sources.newHint",
			"%[1]s is new and off. Enable it with: thinkt config sources enable %[1]s", s.Name))
	}
}

// unregisteredSources returns the available sources missing from srcs (the
// enabled ones). Only IsAvailable is called: a source that is not enabled
// is never opened, so its counts stay empty.
func unregisteredSources(srcs []thinkt.SourceInfo, factories []thinkt.StoreFactory) []thinkt.SourceInfo {
	listed := make(map[thinkt.Source]bool, len(srcs))
	for _, s := range srcs {
		listed[s.Source] = true
	}
	var out []thinkt.SourceInfo
	for _, f := range factories {
		if listed[f.Source()] {
			continue
		}
		if ok, err := f.IsAvailable(); err == nil && ok {
			out = append(out, thinkt.SourceInfo{Source: f.Source(), Name: string(f.Source()), Available: true})
		}
	}
	return out
}

// isSourcesCommand reports whether cmd is `config sources` or below it,
// where the list already shows new sources.
func isSourcesCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c == sourcesCmd {
			return true
		}
	}
	return false
}

// maybeAnnounceNewSources prints the one-time notice for sources found
// after setup and records them, so the notice does not repeat.
func maybeAnnounceNewSources(cmd *cobra.Command) {
	if skipSetup(cmd) || isSourcesCommand(cmd) {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		return
	}
	if announceNewSources(os.Stderr, &cfg, sources.AllFactories()) {
		_ = config.Save(cfg)
	}
}
