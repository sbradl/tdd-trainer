// Package tui renders a session in the terminal: the Quiet coach view by
// default, the Dashboard on Tab.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sbradl/tdd-trainer/internal/app"
	"github.com/sbradl/tdd-trainer/internal/coach"
	"github.com/sbradl/tdd-trainer/internal/results"
	"github.com/sbradl/tdd-trainer/internal/session"
	"github.com/sbradl/tdd-trainer/internal/steps"
)

// Controller is what the keys act on.
type Controller interface {
	Override(steps.Phase)
	ResetBaseline()
	PendingGates() []coach.PendingGate
	// WriteReport writes the session report now and returns its path.
	WriteReport() (string, error)
}

type view int

const (
	quiet view = iota
	dashboard
)

type stepView struct {
	step     steps.Step
	cycle    int
	verdicts []coach.Verdict
}

// Model is the Bubble Tea model.
type Model struct {
	ctl    Controller
	now    func() time.Time
	start  time.Time
	view   view
	width  int
	height int
	frame  int

	phase         steps.Phase
	state         results.TestState
	hasState      bool
	running       bool
	slow          bool
	redInProgress *steps.RedInProgress
	startsRed     bool

	steps   []*stepView
	byN     map[int]*stepView
	cycle   int
	okCount int
	pending []coach.PendingGate
	judge   string

	toast      string
	toastUntil time.Time
	lastErr    string
}

// New creates the model; judgeStatus describes the judge at start.
func New(ctl Controller, judgeStatus string) *Model {
	return &Model{ctl: ctl, now: time.Now, start: time.Now(), byN: map[int]*stepView{}, judge: judgeStatus, width: 80, height: 24}
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Init() tea.Cmd { return tick() }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m, m.key(msg)
	case tickMsg:
		m.frame++
		m.pending = m.ctl.PendingGates()
		return m, tick()

	case session.RunStarted:
		m.running, m.slow = true, false
	case session.SlowRun:
		m.slow = true
	case session.RunFailed:
		m.running = false
		m.lastErr = msg.Err.Error()
	case session.RunDone:
		m.running, m.slow = false, false
		m.state, m.hasState = msg.Outcome.State, true
		m.lastErr = ""
		if !msg.Outcome.State.BuildBroken && len(msg.Outcome.State.Failing) == 0 {
			m.redInProgress = nil
		}

	case steps.Baseline:
		m.startsRed = msg.StartsRed
	case steps.RedInProgress:
		m.redInProgress = &msg
	case steps.PhaseChanged:
		m.showToast("phase set by hand → " + phaseName(msg.Phase))
	case steps.StepDone:
		if msg.Step.Kind == steps.Red {
			m.cycle++
			m.redInProgress = nil
			m.startsRed = false
		}
		sv := &stepView{step: msg.Step, cycle: m.cycle}
		m.steps = append(m.steps, sv)
		m.byN[msg.Step.N] = sv

	case coach.Verdict:
		if sv := m.byN[msg.Step]; sv != nil {
			sv.verdicts = append(sv.verdicts, msg)
		}
		if msg.Level == coach.OK {
			m.okCount++
		}
	case app.PhaseMsg:
		m.phase = msg.Phase
	case app.JudgeMsg:
		switch {
		case msg.Err != nil:
			m.judge = "judge unavailable: " + msg.Err.Error()
		case msg.GPU:
			m.judge = "judge ready (GPU)"
		default:
			m.judge = "judge ready (CPU)"
		}
	case app.ErrorMsg:
		m.lastErr = msg.Err.Error()
	case reportMsg:
		if msg.err != nil {
			m.showToast("report failed: " + msg.err.Error())
		} else {
			m.showToast("report written: " + msg.path)
		}
	}
	return m, nil
}

type reportMsg struct {
	path string
	err  error
}

func (m *Model) key(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "tab":
		m.view = 1 - m.view
	// The controller sends messages back to the program, so it must not be
	// called from Update: that would block the event loop on itself.
	case "r":
		return m.do(func() { m.ctl.Override(steps.PhaseRedInProgress) })
	case "g":
		return m.do(func() { m.ctl.Override(steps.PhaseGreen) })
	case "f":
		return m.do(func() { m.ctl.Override(steps.PhaseRefactor) })
	case "b":
		m.showToast("baseline reset: the next test run is the new starting point")
		return m.do(m.ctl.ResetBaseline)
	case "w":
		return func() tea.Msg {
			p, err := m.ctl.WriteReport()
			return reportMsg{p, err}
		}
	}
	return nil
}

func (m *Model) do(f func()) tea.Cmd {
	return func() tea.Msg {
		f()
		return nil
	}
}

func (m *Model) showToast(s string) {
	m.toast = s
	m.toastUntil = m.now().Add(3 * time.Second)
}

func (m *Model) View() string {
	if m.view == dashboard {
		return m.dashboardView()
	}
	return m.quietView()
}

// phaseName says what the learner is doing, in the cycle's words.
func phaseName(p steps.Phase) string {
	switch p {
	case steps.PhaseGreen:
		return "Green"
	case steps.PhaseRedInProgress:
		return "Red"
	}
	return "Refactor"
}

func testsSummary(st results.TestState, has bool) string {
	switch {
	case !has:
		return "tests not run yet"
	case st.BuildBroken:
		return "build broken"
	case st.Tests == nil && len(st.Failing) == 0:
		return "passing"
	case st.Tests == nil:
		return "failing"
	}
	failed := len(st.Failing)
	passed := len(st.Tests) - failed
	if failed == 0 {
		return fmt.Sprintf("%d passed", passed)
	}
	return fmt.Sprintf("%d passed · %d failed", passed, failed)
}
