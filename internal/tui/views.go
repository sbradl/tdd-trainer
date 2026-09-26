package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

var (
	dim     = lipgloss.NewStyle().Faint(true)
	bold    = lipgloss.NewStyle().Bold(true)
	red     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	green   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	blue    = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	magenta = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	inverse = lipgloss.NewStyle().Reverse(true)
)

const spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

func (m *Model) spinner() string {
	r := []rune(spinnerFrames)
	return string(r[m.frame%len(r)])
}

func phaseStyle(p steps.Phase) lipgloss.Style {
	if p == steps.PhaseRefactor {
		return green
	}
	return red // writing a failing test, or making it pass: tests are red
}

func kindStyle(k steps.Kind) lipgloss.Style {
	switch k {
	case steps.Red:
		return red
	case steps.Green:
		return green
	case steps.Refactor:
		return blue
	}
	return magenta
}

func kindLetter(k steps.Kind) string {
	return map[steps.Kind]string{steps.Red: "R", steps.Green: "G", steps.Refactor: "F", steps.AnomalyStep: "A"}[k]
}

func levelIcon(l coach.Level) string {
	switch l {
	case coach.OK:
		return green.Render("✓")
	case coach.Hint:
		return yellow.Render("➜")
	case coach.Warn:
		return magenta.Render("!")
	}
	return dim.Render("?")
}

// pendingFor counts the gates of step n not judged yet.
func (m *Model) pendingFor(n int) []coach.PendingGate {
	var out []coach.PendingGate
	for _, p := range m.pending {
		if p.Step == n {
			out = append(out, p)
		}
	}
	return out
}

// statusLine: phase, tests, OK count, judging spinner.
func (m *Model) statusLine() string {
	phase := phaseStyle(m.phase).Bold(true).Render("● " + phaseName(m.phase))
	s := fmt.Sprintf(" %s  %s  %s", phase, testsSummary(m.state, m.hasState), green.Render(fmt.Sprintf("✓ %d", m.okCount)))
	if m.running {
		s += dim.Render("  running tests")
	}
	if n := len(m.pending); n > 0 {
		s += dim.Render(fmt.Sprintf("  %s judging %d", m.spinner(), n))
	}
	return s
}

func (m *Model) footer() []string {
	var out []string
	if m.slow {
		out = append(out, yellow.Render(" test run is slow; consider a faster suite for tighter cycles"))
	}
	if m.lastErr != "" {
		out = append(out, red.Render(" error: "+firstLine(m.lastErr)))
	}
	if m.toast != "" && m.now().Before(m.toastUntil) {
		for _, l := range wrap(m.toast, max(20, m.width-4)) {
			out = append(out, " "+inverse.Render(" "+l+" "))
		}
	}
	keys := "Tab dashboard · r/g/f set phase · b baseline · w report · q quit"
	if m.view == dashboard {
		keys = "Tab quiet view · r red  g green  f refactor · b reset baseline · w report · q quit"
	}
	out = append(out, dim.Render(" "+keys+"  ·  "+m.judge))
	return out
}

type card struct {
	level coach.Level
	title string
	text  string
}

func (m *Model) cards() []card {
	var out []card
	if m.startsRed {
		out = append(out, card{coach.Warn, "baseline", "The session starts with failing tests; it is treated as Red in progress. Get to green, or press b once you are."})
	}
	for _, sv := range m.steps {
		if sv.cycle < m.cycle {
			continue // closed cycles collapse into summaries
		}
		for _, v := range sv.verdicts {
			if v.Level == coach.Hint || v.Level == coach.Warn {
				out = append(out, card{v.Level, fmt.Sprintf("step %d · %s · %s", sv.step.N, sv.step.Kind, coach.Label(v.Check)), v.Text})
			}
		}
	}
	if r := m.redInProgress; r != nil {
		text := "The compile error counts as failing: it tells you to create the code under test. Add the smallest stub so the test compiles and fails on its assertion; that completes the Red."
		if !r.BuildBroken {
			text = "The new test errors before its assertion. Make it reach the assertion and fail there."
		}
		out = append(out, card{coach.Hint, "Red in progress", text})
	}
	return out
}

func (m *Model) summaries() []string {
	type sum struct {
		letters   []string
		ok, hints int
		uncertain int
	}
	sums := map[int]*sum{}
	maxCycle := 0
	for _, sv := range m.steps {
		if sv.cycle == 0 || sv.cycle >= m.cycle {
			continue
		}
		s := sums[sv.cycle]
		if s == nil {
			s = &sum{}
			sums[sv.cycle] = s
		}
		s.letters = append(s.letters, kindStyle(sv.step.Kind).Bold(true).Render(kindLetter(sv.step.Kind)))
		for _, v := range sv.verdicts {
			switch v.Level {
			case coach.OK:
				s.ok++
			case coach.Hint, coach.Warn:
				s.hints++
			default:
				s.uncertain++
			}
		}
		maxCycle = max(maxCycle, sv.cycle)
	}
	var out []string
	for c := 1; c <= maxCycle; c++ {
		s := sums[c]
		if s == nil {
			continue
		}
		line := fmt.Sprintf(" %s  %s  %s", dim.Render(fmt.Sprintf("cycle %d", c)), strings.Join(s.letters, " "), green.Render(fmt.Sprintf("%d ✓", s.ok)))
		if s.hints > 0 {
			line += "  " + yellow.Render(fmt.Sprintf("%d hint(s)", s.hints))
		}
		out = append(out, line)
	}
	return out
}

func (m *Model) quietView() string {
	w := max(40, m.width)
	out := []string{m.statusLine(), dim.Render(" " + strings.Repeat("─", w-2))}
	sums := m.summaries()
	out = append(out, sums...)
	if len(sums) > 0 {
		out = append(out, "")
	}
	cards := m.cards()
	for _, c := range cards {
		col := yellow
		if c.level == coach.Warn {
			col = magenta
		}
		out = append(out, " "+col.Render("┌ ")+bold.Render(c.title))
		for _, l := range wrap(c.text, w-6) {
			out = append(out, " "+col.Render("│")+" "+l)
		}
		out = append(out, " "+col.Render("└"))
	}
	if len(cards) == 0 {
		out = append(out, dim.Render(" nothing to say — keep going"))
	}
	return fit(out, m.footer(), m.height)
}

func (m *Model) dashboardView() string {
	w := max(40, m.width)
	var out []string
	bg := map[steps.Phase]string{steps.PhaseGreen: "1", steps.PhaseRedInProgress: "1", steps.PhaseRefactor: "4"}[m.phase]
	banner := " " + strings.ToUpper(phaseName(m.phase)) + " "
	clock := fmt.Sprintf("session %s ", fmtDuration(m.now().Sub(m.start)))
	pad := max(0, w-lipgloss.Width(banner)-lipgloss.Width(clock))
	out = append(out, lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color("15")).Bold(true).
		Render(banner+strings.Repeat(" ", pad)+clock), "")

	var strip []string
	for _, sv := range m.steps {
		mark := green.Render("✓")
		if len(m.pendingFor(sv.step.N)) > 0 {
			mark = dim.Render("…")
		}
		for _, v := range sv.verdicts {
			if v.Level == coach.Hint && mark != magenta.Render("!") {
				mark = yellow.Render("➜")
			}
			if v.Level == coach.Warn {
				mark = magenta.Render("!")
			}
		}
		strip = append(strip, kindStyle(sv.step.Kind).Bold(true).Render(kindLetter(sv.step.Kind))+mark)
	}
	if len(strip) > (w-10)/3 {
		strip = strip[len(strip)-(w-10)/3:]
	}
	out = append(out, " cycle  "+strings.Join(strip, " "), "")

	var cur *stepView
	if len(m.steps) > 0 {
		cur = m.steps[len(m.steps)-1]
	}
	if cur != nil {
		title := cur.step.Kind.String()
		if len(cur.step.NewTests) > 0 {
			title += " — " + strings.Join(cur.step.NewTests, ", ")
		}
		out = append(out, " "+bold.Render("step")+fmt.Sprintf("   %d · %s", cur.step.N, title))
	}
	out = append(out, " "+bold.Render("tests")+"  "+testsSummary(m.state, m.hasState))
	for i, f := range m.state.Failing {
		if i == 3 {
			out = append(out, red.Render(fmt.Sprintf("        … %d more", len(m.state.Failing)-3)))
			break
		}
		out = append(out, red.Render("        "+f.ID))
	}
	if cur != nil {
		out = append(out, "", dim.Render(fmt.Sprintf(" %-28s %-6s %s", "CHECK", "", "VERDICT")))
		for _, v := range cur.verdicts {
			text := v.Text
			if v.Level == coach.Uncertain {
				text = dim.Render(text)
			}
			for i, l := range wrap(text, w-39) {
				if i == 0 {
					out = append(out, fmt.Sprintf(" %-28s %s      %s", coach.Label(v.Check), levelIcon(v.Level), l))
				} else {
					out = append(out, fmt.Sprintf(" %-28s        %s", "", l))
				}
			}
		}
		for _, p := range m.pendingFor(cur.step.N) {
			out = append(out, fmt.Sprintf(" %-28s %s      %s", coach.Label(p.Gate), m.spinner(), dim.Render(fmt.Sprintf("in ~%.0fs", p.ETA.Seconds()))))
		}
	}
	return fit(out, m.footer(), m.height)
}

// fit keeps the body's first lines and the footer at the bottom.
func fit(body, footer []string, height int) string {
	room := max(1, height-len(footer))
	if len(body) > room {
		body = body[:room]
	}
	for len(body) < room {
		body = append(body, "")
	}
	return strings.Join(append(body, footer...), "\n")
}

func wrap(s string, w int) []string {
	if w < 10 {
		w = 10
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, word := range words {
			if line != "" && lipgloss.Width(line)+1+lipgloss.Width(word) > w {
				lines = append(lines, line)
				line = word
				continue
			}
			if line == "" {
				line = word
			} else {
				line += " " + word
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return l
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
