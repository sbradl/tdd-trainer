# Transformation Priority Premise: canonical list and detection

Type: research
Status: resolved
Blocked by: none
Map: ../map.md

## Question

What is the canonical TPP list (Uncle Bob's original and later revisions, variants by others)? Where do sources disagree? Is there any existing work on automatically detecting which transformation a code change applies, language-agnostically or otherwise?

## Answer

No single canonical list: Uncle Bob published three and calls the order tentative and language-specific. No tool anywhere detects which TPP transformation a diff applies, so detection must be built, via tree-sitter/GumTree AST edit rules, an LLM judge, or a hybrid of the two. Closest prior art: Zorro/Besouro (TDD episode inference) and TDD Guard (LLM flags over-implementation).

- v1 (blog, 2010): {}→nil, nil→constant, constant→constant+, constant→scalar, statement→statements, unconditional→if, scalar→array, array→container, statement→recursion, if→while, expression→function, variable→assignment.
- v2 (FibTPP, 2011): adds statement→tail-recursion (before if→while), moves general recursion below if→while, adds (case) last. Says Java should prefer iteration/assignment over recursion.
- v3 (*Clean Craftsmanship*, 2021, latest): {}→Nil, Nil→Constant, Constant→Variable, Unconditional→Selection, Value→List, Selection→Iteration, Statement→Recursion, Value→Mutated Value. Also: combining 2+ transformations in one step hints that a test is missing.
- Disagreements: recursion vs iteration order (even within v3), granularity (v3 drops constant+, statements, expression→function, case), and community additions (Oram's "generalised if predicate").

Details and sources: [../research/tpp-list.md](../research/tpp-list.md)
