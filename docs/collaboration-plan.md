# Claude Code + Codex Collaboration Plan

## 1. Goal

This document defines how Claude Code and Codex should collaborate on the insurance formula engine project without stepping on each other, drifting the schema, or breaking the editor-engine contract.

The main collaboration objective is:

- Keep the formula model as a single source of truth
- Allow parallel work on backend engine and frontend editor
- Reduce merge conflicts and contract mismatches
- Make integration and verification predictable

## 2. Project Reality

This repository is not a generic CRUD application. Its real core is the formula runtime model:

- `FormulaGraph` JSON DAG is the canonical formula representation
- Backend execution depends on node type semantics and edge port semantics
- Frontend editor is responsible for constructing valid graphs
- Text mode must map consistently to the same DAG model
- Versioning wraps the graph with workflow and auditability

Because of that, the most important boundary in the system is not frontend vs backend.
It is:

- formula semantics
- graph serialization
- execution contract

## 3. Collaboration Model

Recommended model:

- Claude Code owns ambiguity reduction
- Codex owns deterministic implementation and integration
- Only one agent performs final integration for a given task slice

Practical interpretation:

- Claude Code should lead when the work involves architecture, schema, workflow rules, or cross-cutting refactors
- Codex should lead when the work involves implementing well-defined behavior, refining modules, adding tests, and checking contract consistency

## 4. Ownership

### 4.1 Claude Code Primary Ownership

Claude Code should usually lead changes in these areas:

- `docs/design.md`
- `docs/requirements.md`
- `backend/internal/domain/`
- `backend/internal/parser/`
- `backend/internal/api/dto.go`
- version workflow design
- schema evolution decisions

Claude Code responsibilities:

- Define or refine DAG schema
- Define node semantics and port semantics
- Define text mode grammar and mapping rules
- Define state machine behavior
- Document intended behavior before implementation spreads

### 4.2 Codex Primary Ownership

Codex should usually lead changes in these areas:

- `frontend/src/components/editor/`
- `frontend/src/utils/graphSerializer.ts`
- `frontend/src/store/`
- backend implementation work already covered by stable contracts
- test creation and regression checks
- consistency review across layers

Codex responsibilities:

- Implement UI and interaction details from the agreed contract
- Preserve serialization consistency between editor and API
- Add focused tests
- Catch schema drift across frontend and backend
- Perform integration verification after changes

### 4.3 Shared Files Requiring Serial Edits

These files should not be edited independently in parallel unless explicitly coordinated:

- `frontend/src/types/formula.ts`
- `backend/internal/domain/formula.go`
- `backend/internal/api/dto.go`
- `frontend/src/utils/graphSerializer.ts`
- `backend/internal/engine/evaluator.go`
- `backend/internal/engine/parallel.go`

Rule:

- One agent proposes the contract change
- The other agent implements dependent updates after the contract is settled

## 5. Source of Truth Rules

To avoid schema drift, the team should follow these rules:

1. Backend domain model is the authoritative runtime contract
   Primary reference: `backend/internal/domain/formula.go`

2. Frontend types must mirror backend semantics, not invent alternate ones
   Mirror file: `frontend/src/types/formula.ts`

3. Graph serialization must preserve runtime-relevant information
   Especially:
   - node type
   - node config
   - source/target IDs
   - sourcePort/targetPort
   - output node IDs
   - layout positions

4. Text mode is not a separate model
   It is another representation of the same DAG semantics

5. Version entities wrap graph snapshots
   Versioning must not mutate the underlying graph contract ad hoc

## 6. Port Semantics Policy

This project depends on edge target port semantics during execution.
Therefore port names must be treated as first-class data, not UI decoration.

Minimum policy:

- Operator nodes must preserve `left` and `right`
- Function nodes must preserve `in` or other named arguments where relevant
- Conditional nodes must preserve:
  - `condition`
  - `conditionRight`
  - `thenValue`
  - `elseValue`
- Table lookup nodes must preserve `key`
- Aggregate nodes must preserve `items`

Implication:

- Default React Flow nodes are not sufficient long term
- Custom node components with named handles should be introduced
- Serializer logic must round-trip handle names without loss

## 7. Task Splitting Strategy

Recommended task splitting is by semantic slice, not by broad layer.

Good splits:

- Slice A: Define conditional node contract
- Slice B: Implement conditional node handles and editor config
- Slice C: Add conditional node execution and tests

- Slice A: Define table lookup graph contract
- Slice B: Implement table lookup UI + serializer
- Slice C: Add API and engine regression tests

Bad splits:

- Claude does “backend”
- Codex does “frontend”

Why this is bad:

- The real coupling is in graph semantics
- Broad layer splits hide contract mismatches until late integration

## 8. Recommended Workflow

For each significant feature, use this sequence:

1. Clarify the contract
   Output:
   - schema decision
   - port names
   - API payload shape
   - acceptance rules

2. Assign write ownership
   Decide which agent is allowed to change which files

3. Implement in bounded patches
   Each patch should be small enough to verify independently

4. Run contract checks
   Compare:
   - backend domain
   - frontend types
   - serializer
   - DTOs

5. Run behavior checks
   Validate:
   - save from editor
   - load existing version
   - calculate
   - validate
   - publish/archive if affected

6. Integrate through one agent
   Final stitching and validation should be centralized

## 9. Suggested Near-Term Backlog

### Priority 1: Lock Graph Contract

Scope:

- confirm node config schemas
- confirm edge port vocabulary
- confirm output node rules
- confirm API request/response examples

Lead:

- Claude Code

Support:

- Codex validates parity across code

### Priority 2: Fix Editor Port Awareness

Scope:

- introduce custom node components
- add named handles
- preserve `sourcePort` and `targetPort`
- add connection validation

Lead:

- Codex

Support:

- Claude Code defines node/port interaction rules

### Priority 3: Make Text Mode a Real Round Trip

Scope:

- define supported expression subset
- implement parse/apply/load flow
- add round-trip tests

Lead:

- Claude Code for grammar and mapping rules
- Codex for integration and tests

### Priority 4: Build Regression Safety Net

Scope:

- engine node tests
- parser round-trip tests
- API contract tests
- editor serialization tests

Lead:

- Codex

Support:

- Claude Code reviews coverage gaps

## 10. Merge and Integration Rules

To reduce conflicts:

- Do not let both agents modify the same schema file simultaneously
- If a contract changes, update dependent files in the same task window
- Prefer short-lived branches and quick integration
- Do not defer type alignment “for later”

Before merging any feature touching formula semantics, verify:

- `backend/internal/domain/formula.go`
- `backend/internal/api/dto.go`
- `frontend/src/types/formula.ts`
- `frontend/src/utils/graphSerializer.ts`
- affected editor components
- affected engine evaluator logic

## 11. Definition of Done

A feature touching the formula system is only done when:

- schema is documented or self-evident in code
- frontend and backend types align
- graph save/load round-trips correctly
- calculation works for the new or changed graph shape
- validation behavior is intentional
- at least one regression test covers the contract

## 12. Communication Template

When handing off between agents, use a short structured note:

- Task:
- Contract decision:
- Owned files:
- Files intentionally not touched:
- Verification performed:
- Remaining risk:

Example:

- Task: Add conditional node support to editor
- Contract decision: conditional node uses `condition`, `conditionRight`, `thenValue`, `elseValue`
- Owned files: `frontend/src/components/editor/*`, `frontend/src/utils/graphSerializer.ts`
- Files intentionally not touched: backend evaluator and domain schema
- Verification performed: local serialization smoke test
- Remaining risk: current backend parser may not yet round-trip nested conditionals

## 13. Bottom Line

This project can be developed collaboratively by Claude Code and Codex very effectively, but only if they collaborate around the formula contract rather than around arbitrary frontend/backend boundaries.

The winning pattern is:

- settle semantics first
- split ownership second
- implement in small slices
- integrate through one agent
- keep schema parity under continuous review
