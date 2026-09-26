// Package judge asks a local language model multiple-choice questions
// about a step's evidence and turns the option probabilities into
// verdicts (SemIf direct mode: one forward pass, softmax over the logits
// of the option letters, no text generated).
package judge

import (
	"fmt"
	"sort"
	"strings"
)

// Part is one labelled piece of evidence.
type Part string

const (
	PartTest       Part = "test"
	PartTranscript Part = "transcript"
	PartSource     Part = "source"
	PartDiff       Part = "diff"
)

var partOrder = []Part{PartTest, PartTranscript, PartSource, PartDiff}

var partLabel = map[Part]string{
	PartTest:       "Test",
	PartTranscript: "Test-runner output",
	PartSource:     "Current source",
	PartDiff:       "Source diff",
}

// Evidence maps parts to their text.
type Evidence map[Part]string

type Option struct{ ID, Desc string }

// Gate is one question about a step.
type Gate struct {
	Name     string
	Question string
	Options  []Option
	Parts    []Part  // evidence the question sees, in order
	Floor    float64 // minimum p for a confident answer
	// Clean is the second probe of a review lens: the answer is "no" only
	// when both probes agree.
	Clean *Gate
}

const (
	Uncertain = "uncertain"
	floor     = 0.8
)

var yesNo = []Option{{"yes", "Yes."}, {"no", "No."}}

// TPPOptions lists the Transformation Priority Premise (Clean
// Craftsmanship, 2021) in order.
var TPPOptions = []Option{
	{"nil", "{} -> Nil: a new function, method or file appears whose body only returns nil/null/None, a zero value, or nothing"},
	{"constant", "Nil -> Constant: a nil/zero value or missing return becomes a literal, an existing literal is edited into another literal, or new code returns fixed literals; no variables, conditions or loops"},
	{"variable", "Constant -> Variable: a literal is replaced, wholly or in part, by a variable, parameter, field, or a value computed from one (for example string concatenation or interpolation with a parameter); no new condition or loop"},
	{"selection", "Unconditional -> Selection: split the execution path with an if, switch, guard, or a new function clause that matches a pattern or guard"},
	{"list", "Value -> List: a single value becomes a list, array or other collection, for example returning [x] instead of x"},
	{"iteration", "Selection -> Iteration: a condition becomes a loop, or a loop over a collection is added"},
	{"recursion", "Statement -> Recursion: the function calls itself, for example on the tail of a list"},
	{"mutation", "Value -> Mutated Value: an existing variable or field is reassigned or changed in place"},
}

var stepSizeOptions = []Option{
	{"constant", "Only string or number literals in the current source need to change or be added; no new variable, field, parameter use, condition or loop."},
	{"simple", "Needs a new variable, field or parameter use, or exactly one new condition (if, switch, guard or pattern clause); no loop, recursion, collection or reassignment."},
	{"complex", "Needs a loop, recursion, a list or other collection built up, a variable reassigned, or a new algorithm."},
}

var refactorEffectOptions = []Option{
	{"improves", "The code is clearly easier to read or change afterwards: clearer names, duplication removed, magic values named, a long function split sensibly."},
	{"neutral", "Readability and design are about the same: cosmetic reordering, formatting, or a change of taste with no clear gain or loss."},
	{"worsens", "The code is harder to read or change afterwards: vaguer names, needless indirection or abstraction, more duplication, denser or trickier expressions."},
}

// Lenses are the review lenses used for missed-refactor hints.
var Lenses = map[string]string{
	"review-ddd":        "Domain-Driven Design: names that drift from the domain's language, business rules living outside the type that owns the data (anemic model), domain concepts passed around as raw strings, numbers or booleans (primitive obsession), missing value objects, logic in the wrong layer, external data shapes leaking into the domain",
	"review-smells":     "code smells (Fowler): duplicated code, long function, long parameter list, feature envy, data clumps, primitive obsession, repeated switches, shotgun surgery, divergent change, speculative generality, message chains, dead code",
	"review-clean-code": "Clean Code (Martin): unclear, cryptic or misleading names, magic numbers, functions doing more than one thing, too many arguments, flag arguments, side effects, comments explaining bad code, inconsistent formatting",
	"review-pragmatic":  "Pragmatic Programmer: duplicated knowledge (DRY), coupling and Law of Demeter violations (reaching through objects), environment-specific values hard-coded in code (hosts, URLs, paths, credentials, limits) that belong in configuration, orthogonality breaks, programming by coincidence, broken windows left unfixed",
	"review-philosophy": "Philosophy of Software Design (Ousterhout): shallow modules, pass-through methods or variables, information leakage (one design decision baked into several places), temporal decomposition, special-case code mixed into general code, conjoined methods, unclear interfaces",
}

// LensProblems lists the concrete problems each lens looks for; the
// "<lens>~which" gate picks the one a diff shows most clearly, so a
// missed-refactor hint can say what to refactor.
var LensProblems = map[string][]Option{
	"review-smells": {
		{"duplicated", "Duplicated code: the same logic or the same literal appears more than once."},
		{"long-function", "Long function: one function does a lot and could be split."},
		{"long-params", "Long parameter list: a function takes many parameters."},
		{"feature-envy", "Feature envy: a function works mostly on another object's data."},
		{"data-clumps", "Data clumps: the same group of values travels together without its own type."},
		{"primitive-obsession", "Primitive obsession: a concept is passed around as a raw string, number or boolean."},
		{"repeated-switch", "Repeated conditionals: the same if/switch on the same value appears in several places, or a chain of special cases grows."},
		{"speculative", "Speculative generality: code or parameters nobody needs yet."},
		{"message-chains", "Message chains: a.b().c().d() reaching through objects."},
		{"dead-code", "Dead code: code that is never used."},
	},
	"review-clean-code": {
		{"names", "Unclear names: cryptic, abbreviated or misleading names."},
		{"magic", "Magic numbers or strings: unexplained literals that deserve a name."},
		{"does-too-much", "A function does more than one thing."},
		{"many-args", "Too many arguments."},
		{"flag-arg", "Flag argument: a boolean parameter switches between two behaviours."},
		{"side-effects", "Hidden side effects."},
		{"comments", "Comments that explain unclear code instead of clarifying it."},
		{"formatting", "Inconsistent formatting."},
	},
	"review-ddd": {
		{"names", "Names drift from the domain's language."},
		{"anemic", "Anemic model: business rules live outside the type that owns the data."},
		{"primitive-obsession", "Domain concepts such as money or IDs passed around as raw strings, numbers or booleans."},
		{"value-object", "A missing value object that would group and validate related values."},
		{"wrong-layer", "Logic in the wrong layer."},
		{"leaking-shapes", "External data shapes leak into the domain."},
	},
	"review-pragmatic": {
		{"dry", "Duplicated knowledge (DRY): the same fact or rule is written in more than one place."},
		{"demeter", "Coupling or Law of Demeter violations: reaching through objects."},
		{"hard-coded", "Environment-specific values (hosts, URLs, paths, credentials, limits) hard-coded instead of configured."},
		{"orthogonality", "Orthogonality break: one change would ripple through unrelated parts."},
		{"coincidence", "Programming by coincidence: code that works for unclear reasons."},
		{"broken-window", "A broken window: sloppy code left unfixed."},
	},
	"review-philosophy": {
		{"shallow", "Shallow module: an interface as complex as what it hides."},
		{"pass-through", "Pass-through methods or variables that only forward to another layer."},
		{"leakage", "Information leakage: one design decision baked into several places."},
		{"temporal", "Temporal decomposition: code split by order of execution instead of by knowledge."},
		{"special-case", "Special-case code mixed into general code."},
		{"conjoined", "Conjoined methods that only make sense together."},
		{"unclear-interface", "An unclear interface."},
	},
}

// Gates holds every gate by name. Any change to a question or option must
// be checked with `tddt judge --regress`.
var Gates = map[string]Gate{}

func add(g Gate) {
	if g.Floor == 0 {
		g.Floor = floor
	}
	Gates[g.Name] = g
}

func init() {
	add(Gate{Name: "red-check", Options: yesNo, Parts: []Part{PartTest, PartTranscript},
		Question: "Does the new test fail for the right reason: the test compiled, ran to its assertion and failed there, expected vs actual? Every other failure is the wrong reason: a build or compile error of any kind (including an undefined or missing symbol), a syntax or parse error, an import error, a crash in shared setup or fixtures, an exception such as NotImplementedError or a null reference raised before the assertion."})
	add(Gate{Name: "cheating", Options: yesNo, Parts: []Part{PartTest, PartDiff},
		Question: "Does the implementation special-case the specific test inputs beyond an acceptable fake-it step, for example branching on exact test values or a lookup table of expected outputs? Returning one literal for one test is acceptable fake-it."})
	add(Gate{Name: "tpp", Options: TPPOptions, Parts: []Part{PartDiff},
		Question: "Which Transformation Priority Premise transformation does this source diff apply? Look at what the added lines do compared with the removed ones. If it applies several, pick the one latest in the list."})
	add(Gate{Name: "multi", Options: yesNo, Parts: []Part{PartDiff},
		Question: "Does this source diff apply two or more different Transformation Priority Premise transformations (for example a new condition and a new loop, or a new variable and a new collection)? A single transformation plus the minimal code it needs is no."})
	add(Gate{Name: "one-behaviour", Options: yesNo, Parts: []Part{PartTest},
		Question: "Does this new test check exactly one behaviour of the code under test? Answer yes only if all its assertions check one outcome of one call or scenario. Different inputs expecting different kinds of results, or several operations each with its own check, are more than one behaviour: answer no."})
	add(Gate{Name: "refactor-effect", Options: refactorEffectOptions, Parts: []Part{PartDiff},
		Question: "This diff is a refactoring: behaviour stays the same. How does it change the design and readability of the code?"})
	add(Gate{Name: "structural", Options: yesNo, Parts: []Part{PartDiff},
		Question: "Is this refactoring diff purely structural, preserving observable behaviour exactly (renames, extractions, moves, inlining, reformatting, rewriting a nested call as a pipeline) with no change in what the code computes? A changed literal, default value, condition or boundary changes behaviour."})
	add(Gate{Name: "step-size", Options: stepSizeOptions, Parts: []Part{PartTest, PartTranscript, PartSource}, Floor: 0.45,
		Question: "The test fails as the runner output shows, against the current source. What is the smallest source change that would make it pass?"})
	for name, focus := range Lenses {
		clean := Gate{Name: name + "~clean", Options: yesNo, Parts: []Part{PartDiff}, Floor: floor,
			Question: fmt.Sprintf("Would a careful reviewer checking only for %s raise at least one real finding on this source diff? A tiny, clearly named change with nothing of that kind is no.", focus)}
		add(Gate{Name: name + "~which", Options: LensProblems[name], Parts: []Part{PartDiff}, Floor: 0.5,
			Question: "A reviewer found at least one of the problems listed in the options in this source diff. Which one does the diff show most clearly?"})
		add(Gate{Name: name, Options: yesNo, Parts: []Part{PartDiff}, Clean: &clean,
			Question: fmt.Sprintf("Does this source diff contain at least one of these problems? Lens: %s. Answer yes if any added code shows one of them, even a small instance.", focus)})
	}
}

// LensNames returns the review lens gate names, sorted.
func LensNames() []string {
	var out []string
	for n := range Lenses {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

const (
	letters      = "ABCDEFGHIJKLMNOP"
	maxPartChars = 6000
	directSystem = "Apply the supplied criterion to the supplied evidence. Choose exactly one listed option. " +
		"Respond with only its uppercase letter, with no explanation or reasoning."
	chatHead = "<|im_start|>system\n" + directSystem + "<|im_end|>\n<|im_start|>user\n"
	chatTail = "<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n"
)

// clip keeps the head and tail of an overlong part.
func clip(s string) string {
	r := []rune(s)
	if len(r) <= maxPartChars {
		return s
	}
	half := maxPartChars / 2
	return string(r[:half]) + "\n[... clipped ...]\n" + string(r[len(r)-half:])
}

// State renders the evidence a gate sees.
func (g Gate) State(ev Evidence) string {
	var out []string
	for _, p := range g.Parts {
		out = append(out, fmt.Sprintf("=== %s ===\n%s", partLabel[p], clip(strings.TrimSpace(ev[p]))))
	}
	return strings.Join(out, "\n\n")
}

// Prompt renders the full chat prompt for a gate.
func (g Gate) Prompt(state string) string {
	return chatHead + userPayload(state, g.Question, g.Options) + chatTail
}

// statePrefix is the prompt up to and including the evidence; prompts of
// all gates sharing this state start with it.
func statePrefix(state string) string { return chatHead + `{"evidence": ` + pyJSONString(state) }

func userPayload(state, question string, opts []Option) string {
	var b strings.Builder
	b.WriteString(`{"evidence": ` + pyJSONString(state) + `, "criterion": ` + pyJSONString(question) + `, "options": [`)
	for i, o := range opts {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`{"letter": "` + string(letters[i]) + `", "description": ` + pyJSONString(o.Desc) + `}`)
	}
	b.WriteString("]}")
	return b.String()
}

// pyJSONString encodes like Python json.dumps(..., ensure_ascii=False).
func pyJSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
