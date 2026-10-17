// Command profile-card regenerates dark_mode.svg / light_mode.svg with live GitHub stats.
//
// Usage: profile-card [output-dir]. Reads ACCESS_TOKEN (a PAT, sees private repos)
// or falls back to GITHUB_TOKEN. Any failure exits non-zero without touching the
// existing SVGs, so the README keeps showing the last good card.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	outDir := "."
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	token := cmp.Or(strings.TrimSpace(os.Getenv("ACCESS_TOKEN")), strings.TrimSpace(os.Getenv("GITHUB_TOKEN")))
	if token == "" {
		fail("error: set ACCESS_TOKEN or GITHUB_TOKEN")
	}
	gh := newGitHubClient(httpTransport(token))
	stats, err := fetchStats(gh, login)
	if ge := (*GitHubError)(nil); errors.As(err, &ge) {
		fail("error: dependency=github-graphql status=%d attempts=%d retries=%d cause=%s", ge.Status, ge.Attempts, gh.retries, ge.Msg)
	} else if err != nil {
		// unexpected response shape (missing field, wrong type): treat as a hard failure, never render partial stats
		fail("error: dependency=github-graphql failure=unexpected-response retries=%d cause=%v", gh.retries, err)
	}

	today := time.Now().UTC()
	// render both before writing either, so a layout error can't leave the two themes out of sync
	cards := make([]string, len(palettes))
	for i, p := range palettes {
		cards[i] = render(p, stats, today)
	}
	for i, p := range palettes {
		target := filepath.Join(outDir, p.name+"_mode.svg")
		if err := writeAtomic(target, cards[i]); err != nil {
			fail("error: write %s: %v", target, err)
		}
	}
	if gh.retries > 0 {
		fmt.Fprintf(os.Stderr, "warn: rendered after %d github retries\n", gh.retries)
	}
}

func writeAtomic(target, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+"*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
