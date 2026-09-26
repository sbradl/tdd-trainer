package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sbradl/tdd-trainer/internal/config"
	"github.com/sbradl/tdd-trainer/internal/preset"
)

// initOpts configure `tddt init`.
type initOpts struct {
	root   string
	preset string // skip detection and use this preset
	yes    bool   // accept the single suggestion without asking
}

// initResult tells the caller where the config went.
type initResult struct {
	dir   string // folder holding the written config; "" if none written
	ready bool   // runnable now: not blank and no setup steps left
}

type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p prompter) ask(q string) (string, error) {
	fmt.Fprint(p.out, q)
	line, err := p.in.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", fmt.Errorf("no answer: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func (p prompter) confirm(q string, def bool) (bool, error) {
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	a, err := p.ask(q + hint)
	if err != nil {
		return false, err
	}
	if a == "" {
		return def, nil
	}
	return strings.HasPrefix(strings.ToLower(a), "y"), nil
}

func (p prompter) pick(q string, options []string) (int, error) {
	for i, o := range options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, o)
	}
	for {
		a, err := p.ask(q + " ")
		if err != nil {
			return 0, err
		}
		if n, err := strconv.Atoi(a); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		fmt.Fprintf(p.out, "Please enter a number from 1 to %d.\n", len(options))
	}
}

func runInit(in io.Reader, out io.Writer, o initOpts) (initResult, error) {
	p := prompter{bufio.NewReader(in), out}
	m, err := choosePreset(p, o)
	if err != nil || m == nil {
		return initResult{}, err
	}

	dir := filepath.Join(o.root, m.Dir)
	path := filepath.Join(dir, config.FileName)
	if _, err := os.Stat(path); err == nil {
		if o.yes {
			return initResult{}, fmt.Errorf("%s already exists", path)
		}
		ok, err := p.confirm(path+" already exists. Overwrite?", false)
		if err != nil || !ok {
			return initResult{}, err
		}
	}
	data, err := m.Preset.Render()
	if err != nil {
		return initResult{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return initResult{}, err
	}
	fmt.Fprintf(out, "Wrote %s.\n", path)
	blank := m.Preset.Name == preset.Blank.Name
	ready := !blank && len(m.Preset.Setup) == 0
	if blank {
		fmt.Fprintln(out, "Fill in the TODOs, then run tddt again.")
	}
	if len(m.Preset.Setup) > 0 {
		fmt.Fprintln(out, "Before the first run:")
		for _, s := range m.Preset.Setup {
			fmt.Fprintln(out, "  -", s)
		}
		fmt.Fprintln(out, "Then run tddt again.")
	}
	return initResult{dir: dir, ready: ready}, nil
}

// choosePreset returns nil when the learner declines everything.
func choosePreset(p prompter, o initOpts) (*preset.Match, error) {
	matches, err := preset.Detect(o.root)
	if err != nil {
		return nil, err
	}

	if o.preset != "" {
		for _, m := range matches {
			if m.Dir == "." && m.Preset.Name == o.preset {
				return &m, nil // keeps detected template vars
			}
		}
		if o.preset == preset.Blank.Name {
			return &preset.Match{Dir: ".", Preset: preset.Blank}, nil
		}
		pr, ok := preset.ByName(o.preset)
		if !ok {
			return nil, fmt.Errorf("unknown preset %q (want one of %s)", o.preset, presetNames())
		}
		return &preset.Match{Dir: ".", Preset: pr}, nil
	}

	switch {
	case len(matches) == 1:
		if o.yes {
			return &matches[0], nil
		}
		ok, err := p.confirm(fmt.Sprintf("Detected %s. Write %s?", matches[0], config.FileName), true)
		if err != nil || ok {
			return &matches[0], err
		}
	case len(matches) > 1:
		if o.yes {
			return nil, errors.New("several projects detected; pick one with --preset or run init in its folder")
		}
		opts := make([]string, len(matches))
		for i, m := range matches {
			opts[i] = m.String()
		}
		fmt.Fprintln(p.out, "Detected several projects; one session watches one project:")
		i, err := p.pick("Which one?", opts)
		if err != nil {
			return nil, err
		}
		return &matches[i], nil
	default:
		if o.yes {
			return nil, errors.New("no project detected; pick one with --preset")
		}
		fmt.Fprintln(p.out, "No known project type detected.")
	}

	all := append(preset.All(), preset.Blank)
	opts := make([]string, len(all))
	for i, pr := range all {
		opts[i] = pr.Title
	}
	fmt.Fprintln(p.out, "Choose a preset:")
	i, err := p.pick("Which one?", opts)
	if err != nil {
		return nil, err
	}
	return &preset.Match{Dir: ".", Preset: all[i]}, nil
}

func presetNames() string {
	var names []string
	for _, p := range preset.All() {
		names = append(names, p.Name)
	}
	return strings.Join(append(names, preset.Blank.Name), ", ")
}
