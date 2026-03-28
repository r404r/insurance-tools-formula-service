# Next Steps

## Goal

Turn the current formula-service repository from a solid prototype into a stable collaborative development track for Claude Code + Codex.

This plan assumes:

- the backend core is already largely in place
- the frontend editor is functional but still semantically thin
- the highest current risk is contract drift between editor and engine

## Guiding Priority

The next phase should optimize for correctness of the formula contract before breadth of features.

That means the immediate order is:

1. stabilize graph semantics
2. strengthen editor serialization
3. establish round-trip and regression testing
4. then expand product features

## Phase 1: Contract Lock

Target outcome:

- one clear contract for graph nodes, edge ports, outputs, and calculation payloads

Tasks:

- Review `backend/internal/domain/formula.go` and confirm each node config schema
- Review `frontend/src/types/formula.ts` and remove drift from backend semantics
- Review `backend/internal/api/dto.go` and ensure request/response fields match actual frontend usage
- Add concrete graph examples to `docs/design.md`
- Document the supported edge target ports by node type
- Document which node types are currently production-ready vs planned

Suggested ownership:

- Claude Code:
  - contract clarification
  - docs updates
  - schema decisions
- Codex:
  - parity check across code
  - fix type mismatches
  - flag inconsistent API usage

Definition of done:

- backend and frontend graph types match
- documented port vocabulary exists
- no known schema ambiguity remains for existing node types

## Phase 2: Editor Port Semantics

Target outcome:

- frontend editor can express the same edge semantics the backend engine expects

Tasks:

- Introduce custom React Flow node components
- Add named handles for operator/function/conditional/tableLookup/aggregate nodes
- Preserve `sourceHandle` and `targetHandle` when creating edges
- Update `frontend/src/utils/graphSerializer.ts` to round-trip port names
- Add connection validation rules in the editor
- Prevent saving malformed graphs when required ports are missing
- Improve node labels so config changes are reflected visually

Suggested ownership:

- Claude Code:
  - define handle strategy and allowed connections
- Codex:
  - implement custom nodes
  - implement serializer changes
  - implement save-time validation

Definition of done:

- operator nodes reliably preserve `left` and `right`
- function nodes preserve intended input handles
- conditional nodes preserve all required input ports
- saved graph can be loaded and calculated without port loss

## Phase 3: Text Mode Round Trip

Target outcome:

- text mode becomes a true alternate representation of the same graph

Tasks:

- Confirm the supported expression subset in parser and serializer
- Define which node types are allowed in text mode in phase 1
- Add or expose API endpoints for DAG-to-text and text-to-DAG if needed
- Connect `TextEditor` to real backend parse/serialize flow
- Show parser/validation errors in the UI
- Add round-trip tests for representative formulas

Suggested ownership:

- Claude Code:
  - grammar and mapping rules
  - supported scope decisions
- Codex:
  - UI integration
  - API wiring
  - tests

Definition of done:

- a supported formula can move visual -> text -> visual without semantic drift
- parse failures are understandable from the UI

## Phase 4: Regression Safety Net

Target outcome:

- feature work becomes safer and easier to parallelize

Tasks:

- Add backend tests for all currently supported node types
- Add parser precedence tests
- Add version state transition tests
- Add API tests for calculate, validate, and versions
- Add frontend serialization tests for graph save/load
- Add at least one E2E happy-path test:
  - login
  - create formula
  - add nodes
  - save version
  - calculate

Suggested ownership:

- Codex:
  - implement tests
  - create fixtures
- Claude Code:
  - review missing coverage
  - expand risk scenarios

Definition of done:

- core formula workflow has automated coverage
- schema regressions are caught early

## Phase 5: Product Feature Expansion

Only start this phase after phases 1-4 are in good shape.

Candidate work:

- version diff view
- lookup table management UI
- insurance domain formula templates
- PostgreSQL store
- MySQL store
- richer node visuals
- performance/load testing

Recommended order:

1. version diff view
2. lookup table management UI
3. domain templates
4. database expansion
5. load testing

## Suggested Two-Week Execution Plan

### Week 1

Focus:

- lock the contract
- fix editor serialization semantics

Tasks:

- finalize node config and port vocabulary
- align frontend types with backend domain
- implement custom node handles
- update graph serializer
- add save-time validation

Expected output:

- editor can create valid executable graphs for core node types

### Week 2

Focus:

- make text mode real
- build safety net

Tasks:

- wire text parsing/serialization flow
- add backend contract tests
- add frontend serialization tests
- add one end-to-end happy path

Expected output:

- one dependable end-to-end formula workflow

## Immediate Task Candidates

If starting right away, the best first tickets are:

1. Define edge port vocabulary by node type in `docs/design.md`
2. Align `frontend/src/types/formula.ts` with backend domain and evaluator behavior
3. Implement custom editor nodes with named handles
4. Update `frontend/src/utils/graphSerializer.ts` to preserve handles
5. Add backend tests for operator/function/conditional/tableLookup node execution

## Coordination Rules

While executing this plan:

- do not change schema silently
- do not modify shared schema files in parallel without coordination
- prefer small patches that each leave the system runnable
- attach verification notes to each handoff

## Success Criteria

This phase is successful when:

- Claude Code and Codex can work in parallel without frequent merge collisions
- the graph contract is stable enough that frontend and backend stop drifting
- the editor produces graphs the engine can execute reliably
- text mode and visual mode are no longer separate partial implementations
- future feature work has test coverage to build on
