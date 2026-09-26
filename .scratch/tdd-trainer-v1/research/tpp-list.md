# Transformation Priority Premise: canonical list and detection

Research for [issues/03-tpp-list.md](../issues/03-tpp-list.md). Checked 2026-09-26.

## TL;DR

- There is no single canonical list. Robert C. Martin (Uncle Bob) has published **three** versions: the original 12-item blog list (Dec 2010), a 14-item revision (Feb 2011, adds tail-recursion and `(case)`), and a condensed 8-item list in *Clean Craftsmanship* (2021). The 2021 book is his latest word.
- Martin himself treats the list as tentative: probably incomplete, probably mis-ordered, and dependent on the language (Java should prefer iteration over recursion).
- No existing tool detects which TPP transformation a code change applies, for any language. The nearest work is TDD process-conformance recognisers (Zorro, Besouro), an LLM-based TDD enforcer (TDD Guard), syntax-aware diff engines (GumTree), and refactoring miners. All are building blocks, not TPP classifiers.

## 1. Martin's lists

### v1: original blog post (19 Dec 2010)

Source: "The Transformation Priority Premise", https://blog.cleancoder.com/uncle-bob/2013/05/27/TheTransformationPriorityPremise.html. The post is dated December 19 2010 and was migrated to the current blog under a 2013 URL.

1. `({}→nil)` no code at all → code that employs nil
2. `(nil→constant)`
3. `(constant→constant+)` a simple constant to a more complex constant
4. `(constant→scalar)` replacing a constant with a variable or an argument
5. `(statement→statements)` adding more unconditional statements
6. `(unconditional→if)` splitting the execution path
7. `(scalar→array)`
8. `(array→container)`
9. `(statement→recursion)`
10. `(if→while)`
11. `(expression→function)` replacing an expression with a function or algorithm
12. `(variable→assignment)` replacing the value of a variable

Key claims from the same post:
- "There are likely others."
- Transformations change behaviour, while refactorings keep it. Each transformation goes "from something specific to something more generic".
- The list is "roughly ordered … by their complexity". When making a test pass, prefer transformations higher on the list. When choosing the next test, pick one that can be passed with a higher-priority transformation. Following this avoids the "impasse" (for example in the word-wrap kata).
- Open questions Martin lists himself: "Are there other transformations? (almost certainly)", "Are these the right transformations? (probably not)", "Is there really a priority? (I think so, but it might be more complicated than a simple ordinal sequence)", "Can it be quantified? (I have no idea)", "Is the priority order presented in this blog correct? (not likely)", "Can they be formalized? (That's the holy grail!)".
- He names as a future possibility "Tool support for suggesting transformations that follow priority order." That is essentially what this project would build.

### v2: "Fib. The T-P Premise" (2 Feb 2011)

Source: https://blog.cleancoder.com/uncle-bob/2013/05/27/FibTPP.html

The list as quoted in the post:

1. `({}→nil)`
2. `(nil→constant)`
3. `(constant→constant+)`
4. `(constant→scalar)`
5. `(statement→statements)`
6. `(unconditional→if)`
7. `(scalar→array)`
8. `(array→container)`
9. **`(statement→tail-recursion)`** (new)
10. `(if→while)`
11. `(statement→recursion)` (now below if→while)
12. `(expression→function)`
13. `(variable→assignment)`
14. **`(case)`** adding a case (or else) to an existing switch or if (new, "always the last option to choose")

Also from this post:
- "the priority list is language specific. In Java, for example, we might move (if→while) and (variable→assignment) above (statement→tail-recursion) so that iteration is always preferred above recursion, and assignment is preferred above parameter passing."
- "the transformations and their priorities are a way to encode a particular programming style." He expects that teams may adjust the list, but that most low-level decisions stay the same across languages.

The TPP article on Wikipedia (https://en.wikipedia.org/wiki/Transformation_Priority_Premise) reproduces v2 and renames item 11 to "statement → non-tail-recursion". Corey Haines' 2012 gist (https://gist.github.com/3151661) is v1 with `case` appended at the end. It is a hybrid that drops tail-recursion.

### v3: *Clean Craftsmanship* (Addison-Wesley, 2021), ch. 4 "Test Design"

Sources: publisher page https://www.informit.com/store/clean-craftsmanship-disciplines-standards-and-ethics-9780136915713. The book text was checked via the table of contents at https://www.oreilly.com/library/view/clean-craftsmanship-disciplines/9780136915805/.

The summary list ("something like the following order"):

1. `{} → Nil`
2. `Nil → Constant`
3. `Constant → Variable`
4. `Unconditional → Selection` ("adds an if statement, or the equivalent"; the predicate must not be specific to the test, e.g. `n>1`, not `n==2`)
5. `Value → List` (a single-value variable becomes an array or container)
6. `Selection → Iteration` ("selection is merely degenerate iteration")
7. `Statement → Recursion`
8. `Value → Mutated Value` (mutating a variable, e.g. accumulating in a loop or `list.set`; plain initialisation does not count)

Also from the book:
- The list is framed as "only a premise. I have no mathematical proof".
- The order is not a sequence you must walk through; programmers may jump straight from Nil to a Selection of two constants. But "if you are tempted to pass a test by combining two or more transformations, you may be missing one or more tests. Try to find a test that can be passed by using just one of these transformations." At a fork, choose the branch that uses the higher transformation.
- Following the order "will lead you to implement solutions using the functional programming style".
- In the sort kata, choosing Value → Mutated Value led to bubble sort, while choosing Unconditional → Selection led to quicksort.
- Transformations are defined as "small changes to the code that change behavior and simultaneously generalize the solution". The Green step is "transformative", the Refactor step "restorative".
- The book is internally inconsistent. Its section headings (and the TOC) run `… Value → List, Statement → Recursion, Selection → Iteration, Value → Mutated Value` and have no Constant → Variable heading. Its summary list puts Iteration above Recursion and includes Constant → Variable.

### How the versions map onto each other

| v1 (2010) | v2 (2011) | v3 (2021) |
|---|---|---|
| {}→nil | {}→nil | {}→Nil |
| nil→constant | nil→constant | Nil→Constant |
| constant→constant+ | constant→constant+ | *(dropped)* |
| constant→scalar | constant→scalar | Constant→Variable |
| statement→statements | statement→statements | *(dropped)* |
| unconditional→if | unconditional→if | Unconditional→Selection |
| scalar→array, array→container | same | Value→List (merged) |
| — | statement→tail-recursion | *(dropped)* |
| if→while (after recursion) | if→while (between tail and general recursion) | Selection→Iteration (before Recursion in the summary list) |
| statement→recursion | statement→recursion | Statement→Recursion |
| expression→function | expression→function | *(dropped)* |
| variable→assignment | variable→assignment | Value→Mutated Value |
| — | (case), last | *(dropped / folded into Selection)* |

## 2. Where sources disagree

1. **Recursion vs iteration order.** In v1, recursion comes before while. v2 splits it: tail-recursion, then while, then general recursion. It also says Java should put while and assignment above recursion. v3's summary puts Iteration before Recursion, but its section order is the reverse. For a coach, the order of this pair should be treated as configurable per language and style.
2. **Granularity.** v1 and v2 have fine-grained steps (constant+, statement→statements, scalar→array→container, expression→function, case). v3 merges or drops these. `expression→function` is the vaguest v1/v2 item: Martin himself uses it for "adding is a function" (`length + 1`).
3. **Whether `(case)` / else-if is distinct from `unconditional→if`.** v2 says it is, and ranks it last. v3 does not list it separately.
4. **Community extensions.** Tom Oram (2018, https://tomphp.github.io/blog/tdd-a-new-transformation) proposes `(if expression → generalised if expression)`, generalising a predicate, placed at "5.5" before unconditional→if. Martin's v3 text about keeping predicates general (`n>1` instead of `n==2`) touches the same idea.
5. **Wikipedia and secondary write-ups** mostly copy v2 (Wikipedia renames item 11 "non-tail-recursion") or v1 plus case (Haines). Almost none reflect v3.
6. **Ordinal vs partial order.** Martin himself doubts that the priority is a simple total order (v1 conclusions, v3 "something like the following order").

## 3. Existing work on detecting transformations

**TPP classifiers: none found.** Searches of GitHub, the web and academic sources turned up no tool, paper or dataset that labels a code diff with a TPP transformation, whether language-agnostic or language-specific. What exists instead is teaching material (e.g. `kuerm/tpp_by_example`, https://github.com/kuerm/tpp_by_example) and Martin's own call for "tool support".

Adjacent work the project can reuse:

- **TDD process recognition (step inference, not TPP).**
  - Zorro: Kou, Johnson, Erdogmus, "Operational definition and automated inference of test-driven development with Zorro", *Automated Software Engineering* 17 (2010), https://www.researchgate.net/publication/220136051. It classifies IDE event streams into TDD episodes by heuristic rules.
  - Besouro: Becker, Pedroso, Pimenta, Jeffries, *IST* 2015, https://www.sciencedirect.com/science/article/abs/pii/S0950584914001426. It is a standalone Eclipse plugin with live feedback: https://github.com/brunopedroso/besouro.
  - Both judge process conformance (test-first, episode types) and ignore how simple the Green code is. Relevant to red/green/refactor inference.
- **LLM-based TDD enforcement.** TDD Guard (https://github.com/nizos/tdd-guard) is a Claude Code hook. It uses an LLM plus test-reporter output (JS/TS, Python, PHP, Go, Rust, Ruby) to block implementation without a failing test and to flag "code beyond current test requirements". It judges over-implementation, but does not name TPP transformations. It is the closest existing analogue to a Green-step "simplest code" judge.
- **TPP as a generation prior (not detection).** Dong et al., "From I/O to Code with Discovery Agent", arXiv 2605.15334 (May 2026), https://arxiv.org/abs/2605.15334. It uses TPP to bias LLM program synthesis toward the simplest hypothesis: constants → conditionals → iteration.
- **Syntax-aware diffing (building block for rule-based detection).** GumTree (Falleri et al., ASE 2014, https://github.com/GumTreeDiff/gumtree) produces AST edit scripts (insert, delete, update, move) and has a tree-sitter backend, so it covers many languages. A rule-based detector could map edit-script patterns to transformations. For example, a literal replaced by an identifier or parameter is constant→variable; an inserted `if` node that wraps existing statements is unconditional→selection; an `if` node that becomes `while`/`for` is selection→iteration; a new call to the enclosing function is recursion; a new assignment to an already-bound name is mutation.
- **Refactoring miners** are the structural twin of this problem: they detect behaviour-preserving transformations from diffs. Examples are RefactoringMiner (Tsantalis et al., Java) and RefDiff 2.0 (Brito & Valente, multi-language via plugins). They show that diff classification by AST rules works well in practice, but they target refactorings, not TPP.

## 4. Implications for the Judge (not decisions)

- Ship a configurable list. v3 (8 items) makes a good coarse default for learner-facing names. The v2 fine-grained items (constant+, statement→statements, container, case, tail-recursion) are useful as sub-labels or hints. Iteration vs recursion should be a per-language switch, as Martin himself suggests.
- The verdict Martin's own texts support is: "Could this test have been passed with fewer transformations, or a higher one?" plus "Did the Green step combine several transformations? Then a test may be missing" (v3). It is not "exactly which item was applied".
- Detection has to be built. Options: (a) tree-sitter/GumTree edit-script heuristics, which are deterministic, need per-grammar node mappings, and are language-agnostic only up to that mapping; (b) an LLM or small-model judge given the diff, the failing test and the list, as TDD Guard does; (c) a hybrid, using AST features as evidence for the model. There is no prior benchmark, so the evals would need to build their own labelled kata diffs, e.g. prime factors, word wrap, stack, bowling, fib, sort from Martin's texts.

## Sources

- R. C. Martin, "The Transformation Priority Premise" (2010-12-19): https://blog.cleancoder.com/uncle-bob/2013/05/27/TheTransformationPriorityPremise.html
- R. C. Martin, "Fib. The T-P Premise" (2011-02-02): https://blog.cleancoder.com/uncle-bob/2013/05/27/FibTPP.html
- R. C. Martin, "Transformation Priority and Sorting" (2011-01-01, comic): https://blog.cleancoder.com/uncle-bob/2013/05/27/TransformationPriorityAndSorting.html
- R. C. Martin, *Clean Craftsmanship* (2021), ch. 4: https://www.oreilly.com/library/view/clean-craftsmanship-disciplines/9780136915805/
- R. C. Martin, Clean Code video ep. 24 "Transformation Priority Premise" (paywalled, not reviewed): https://cleancoders.com/episode/clean-code-episode-24-p1
- Wikipedia, TPP: https://en.wikipedia.org/wiki/Transformation_Priority_Premise
- C. Haines gist (2012): https://gist.github.com/3151661
- T. Oram, "TDD — A New Transformation" (2018): https://tomphp.github.io/blog/tdd-a-new-transformation
- Zorro: https://www.researchgate.net/publication/220136051 ; Besouro: https://www.sciencedirect.com/science/article/abs/pii/S0950584914001426
- TDD Guard: https://github.com/nizos/tdd-guard
- Dong et al. 2026: https://arxiv.org/abs/2605.15334
- GumTree: https://github.com/GumTreeDiff/gumtree
