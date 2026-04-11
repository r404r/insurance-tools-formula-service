package engine

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// parallelThreshold is the minimum number of nodes in a level before
// the executor will launch goroutines instead of evaluating sequentially.
const parallelThreshold = 4

// ExecutionPlan is a pre-computed plan for evaluating a formula graph.
type ExecutionPlan struct {
	// Levels contains node IDs grouped by topological level.
	Levels [][]string
	// Graph is the original FormulaGraph.
	Graph *domain.FormulaGraph
	// DAG is the validated DAG built from Graph.
	DAG *DAG
}

type SubFormulaRunner func(ctx context.Context, node *domain.FormulaNode, nodeInputs map[string]Decimal, seedInputs map[string]Decimal) (Decimal, error)

// LoopRunner executes a loop node by iterating over a bounded integer range,
// calling a sub-formula for each iteration, and aggregating the results.
type LoopRunner func(ctx context.Context, node *domain.FormulaNode, nodeInputs map[string]Decimal, seedInputs map[string]Decimal) (Decimal, error)

// Executor evaluates a formula graph using level-based parallelism.
type Executor struct {
	workers          int
	precision        PrecisionConfig
	evaluator        *Evaluator
	subFormulaRunner SubFormulaRunner
	loopRunner       LoopRunner
}

// NewExecutor creates an Executor with the given worker count and precision
// configuration. If workers <= 0 it defaults to 1. The optional resolver is
// passed through to the evaluator so that NodeTableAggregate can scan a
// table at evaluation time; pass nil if no aggregate nodes will run.
func NewExecutor(workers int, precision PrecisionConfig, resolver TableResolver, subFormulaRunner SubFormulaRunner, loopRunner LoopRunner) *Executor {
	if workers <= 0 {
		workers = 1
	}
	return &Executor{
		workers:          workers,
		precision:        precision,
		evaluator:        NewEvaluator(precision, resolver),
		subFormulaRunner: subFormulaRunner,
		loopRunner:       loopRunner,
	}
}

// BuildPlan builds a DAG from the graph, validates it, and returns a
// level-based execution plan.
func BuildPlan(graph *domain.FormulaGraph) (*ExecutionPlan, error) {
	dag, err := BuildDAG(graph)
	if err != nil {
		return nil, fmt.Errorf("build execution plan: %w", err)
	}
	levels := dag.TopologicalLevels()
	return &ExecutionPlan{
		Levels: levels,
		Graph:  graph,
		DAG:    dag,
	}, nil
}

// Execute runs the execution plan with the provided input values. It seeds
// input variables, then evaluates each topological level -- in parallel for
// large levels, sequentially for small ones.
//
// The returned map contains results keyed by node ID for every node in the
// graph.
func (ex *Executor) Execute(ctx context.Context, plan *ExecutionPlan, inputs map[string]Decimal) (map[string]Decimal, error) {
	results := &sync.Map{}

	// Seed all provided inputs into the results map.
	for k, v := range inputs {
		results.Store(k, v)
	}

	for _, level := range plan.Levels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if len(level) >= parallelThreshold && ex.workers > 1 {
			if err := ex.executeParallel(ctx, plan, level, results, inputs); err != nil {
				return nil, err
			}
		} else {
			if err := ex.executeSequential(ctx, plan, level, results, inputs); err != nil {
				return nil, err
			}
		}
	}

	return syncMapToMap(results), nil
}

// executeSequential evaluates all nodes in the level one at a time.
func (ex *Executor) executeSequential(ctx context.Context, plan *ExecutionPlan, level []string, results *sync.Map, seedInputs map[string]Decimal) error {
	for _, nodeID := range level {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ex.evaluateAndStore(ctx, plan, nodeID, results, seedInputs); err != nil {
			return err
		}
	}
	return nil
}

// executeParallel evaluates all nodes in the level concurrently, limited by
// the executor's worker count.
func (ex *Executor) executeParallel(ctx context.Context, plan *ExecutionPlan, level []string, results *sync.Map, seedInputs map[string]Decimal) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(ex.workers)

	for _, nodeID := range level {
		id := nodeID
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return ex.evaluateAndStore(ctx, plan, id, results, seedInputs)
		})
	}
	return g.Wait()
}

// evaluateAndStore resolves the inputs for a node from the results map,
// evaluates it, and stores the result.
func (ex *Executor) evaluateAndStore(ctx context.Context, plan *ExecutionPlan, nodeID string, results *sync.Map, seedInputs map[string]Decimal) error {
	node := plan.DAG.Node(nodeID)
	if node == nil {
		return fmt.Errorf("node %s not found in DAG", nodeID)
	}

	// For variable nodes, the value may already be seeded.
	if node.Type == domain.NodeVariable {
		// Try to evaluate from the seeded inputs.
		nodeInputs, err := ex.resolveInputs(plan, nodeID, results)
		if err != nil {
			return err
		}
		val, err := ex.evaluator.EvaluateNode(node, nodeInputs)
		if err != nil {
			return fmt.Errorf("evaluate node %s: %w", nodeID, err)
		}
		results.Store(nodeID, val)
		return nil
	}

	nodeInputs, err := ex.resolveInputs(plan, nodeID, results)
	if err != nil {
		return err
	}

	var val Decimal
	if node.Type == domain.NodeSubFormula && ex.subFormulaRunner != nil {
		val, err = ex.subFormulaRunner(ctx, node, nodeInputs, seedInputs)
		if err != nil {
			return fmt.Errorf("evaluate node %s: %w", nodeID, err)
		}
	} else if node.Type == domain.NodeLoop && ex.loopRunner != nil {
		val, err = ex.loopRunner(ctx, node, nodeInputs, seedInputs)
		if err != nil {
			return fmt.Errorf("evaluate node %s: %w", nodeID, err)
		}
	} else {
		val, err = ex.evaluator.EvaluateNode(node, nodeInputs)
		if err != nil {
			return fmt.Errorf("evaluate node %s: %w", nodeID, err)
		}
	}

	results.Store(nodeID, val)
	return nil
}

// resolveInputs gathers the input values for a node from the results map
// using the edge definitions. The returned map is keyed by target port name.
func (ex *Executor) resolveInputs(plan *ExecutionPlan, nodeID string, results *sync.Map) (map[string]Decimal, error) {
	incoming := plan.DAG.IncomingEdges(nodeID)
	nodeInputs := make(map[string]Decimal, len(incoming))

	for _, edge := range incoming {
		val, ok := results.Load(edge.Source)
		if !ok {
			return nil, fmt.Errorf("node %s: dependency %s not yet computed", nodeID, edge.Source)
		}
		d, ok := val.(Decimal)
		if !ok {
			return nil, fmt.Errorf("node %s: dependency %s has non-decimal value", nodeID, edge.Source)
		}
		port := edge.TargetPort
		if port == "" {
			port = "in"
		}
		nodeInputs[port] = d
	}

	// For variable nodes, also include all seeded inputs so the evaluator
	// can look up by variable name.
	// For tableLookup nodes, include pre-loaded "table:*" entries so the
	// evaluator can resolve table data by composite key.
	node := plan.DAG.Node(nodeID)
	if node != nil && node.Type == domain.NodeVariable {
		results.Range(func(key, value any) bool {
			k, ok1 := key.(string)
			v, ok2 := value.(Decimal)
			if ok1 && ok2 {
				if _, exists := nodeInputs[k]; !exists {
					nodeInputs[k] = v
				}
			}
			return true
		})
	}
	if node != nil && node.Type == domain.NodeTableLookup {
		results.Range(func(key, value any) bool {
			k, ok1 := key.(string)
			v, ok2 := value.(Decimal)
			if ok1 && ok2 && len(k) > 6 && k[:6] == "table:" {
				nodeInputs[k] = v
			}
			return true
		})
	}

	return nodeInputs, nil
}

// syncMapToMap converts a sync.Map to a plain map[string]Decimal.
func syncMapToMap(sm *sync.Map) map[string]Decimal {
	out := make(map[string]Decimal)
	sm.Range(func(key, value any) bool {
		if k, ok := key.(string); ok {
			if v, ok := value.(Decimal); ok {
				out[k] = v
			}
		}
		return true
	})
	return out
}
