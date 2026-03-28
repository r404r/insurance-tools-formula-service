package engine

import (
	"fmt"
	"strings"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// DAG is a directed acyclic graph built from a FormulaGraph. It stores
// adjacency information and enables topological ordering for evaluation.
type DAG struct {
	// forward maps nodeID -> list of successor node IDs.
	forward map[string][]string
	// reverse maps nodeID -> list of predecessor node IDs.
	reverse map[string][]string
	// inDegree tracks the number of incoming edges for each node.
	inDegree map[string]int
	// nodes maps nodeID -> FormulaNode for fast lookup.
	nodes map[string]*domain.FormulaNode
	// edges stores the original edge list for port-level lookups.
	edges []domain.FormulaEdge
	// outputs is the set of output node IDs declared on the graph.
	outputs map[string]bool
}

// BuildDAG constructs a DAG from a FormulaGraph, validates that the graph
// is acyclic, and returns the resulting structure.
func BuildDAG(graph *domain.FormulaGraph) (*DAG, error) {
	d := &DAG{
		forward:  make(map[string][]string),
		reverse:  make(map[string][]string),
		inDegree: make(map[string]int),
		nodes:    make(map[string]*domain.FormulaNode),
		edges:    graph.Edges,
		outputs:  make(map[string]bool),
	}

	// Index nodes.
	for i := range graph.Nodes {
		n := &graph.Nodes[i]
		d.nodes[n.ID] = n
		d.forward[n.ID] = nil
		d.reverse[n.ID] = nil
		d.inDegree[n.ID] = 0
	}

	// Mark outputs.
	for _, id := range graph.Outputs {
		d.outputs[id] = true
	}

	// Build adjacency from edges.
	for _, e := range graph.Edges {
		if _, ok := d.nodes[e.Source]; !ok {
			return nil, fmt.Errorf("edge references unknown source node %q", e.Source)
		}
		if _, ok := d.nodes[e.Target]; !ok {
			return nil, fmt.Errorf("edge references unknown target node %q", e.Target)
		}
		d.forward[e.Source] = append(d.forward[e.Source], e.Target)
		d.reverse[e.Target] = append(d.reverse[e.Target], e.Source)
		d.inDegree[e.Target]++
	}

	// Validate acyclicity via a trial topological sort.
	if err := d.validateAcyclic(); err != nil {
		return nil, err
	}

	return d, nil
}

// validateAcyclic performs Kahn's algorithm and returns an error listing the
// cycle participants if a cycle is detected.
func (d *DAG) validateAcyclic() error {
	tmpDeg := make(map[string]int, len(d.inDegree))
	for k, v := range d.inDegree {
		tmpDeg[k] = v
	}

	queue := make([]string, 0)
	for id, deg := range tmpDeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, succ := range d.forward[node] {
			tmpDeg[succ]--
			if tmpDeg[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}

	if visited != len(d.nodes) {
		// Collect nodes that are part of a cycle.
		cycleNodes := make([]string, 0)
		for id, deg := range tmpDeg {
			if deg > 0 {
				cycleNodes = append(cycleNodes, id)
			}
		}
		return fmt.Errorf("cycle detected involving nodes: %s", strings.Join(cycleNodes, ", "))
	}
	return nil
}

// TopologicalLevels returns nodes grouped by execution level using Kahn's
// algorithm. Nodes within the same level have no dependencies on each other
// and can be evaluated in parallel.
func (d *DAG) TopologicalLevels() [][]string {
	tmpDeg := make(map[string]int, len(d.inDegree))
	for k, v := range d.inDegree {
		tmpDeg[k] = v
	}

	currentLevel := make([]string, 0)
	for id, deg := range tmpDeg {
		if deg == 0 {
			currentLevel = append(currentLevel, id)
		}
	}

	var levels [][]string
	for len(currentLevel) > 0 {
		levels = append(levels, currentLevel)
		nextLevel := make([]string, 0)
		for _, node := range currentLevel {
			for _, succ := range d.forward[node] {
				tmpDeg[succ]--
				if tmpDeg[succ] == 0 {
					nextLevel = append(nextLevel, succ)
				}
			}
		}
		currentLevel = nextLevel
	}
	return levels
}

// InputNodes returns the IDs of nodes with zero incoming edges -- these are
// the input variables or constants that must be seeded before execution.
func (d *DAG) InputNodes() []string {
	result := make([]string, 0)
	for id, deg := range d.inDegree {
		if deg == 0 {
			result = append(result, id)
		}
	}
	return result
}

// OutputNodes returns the IDs of the declared output nodes for this graph.
func (d *DAG) OutputNodes() []string {
	result := make([]string, 0, len(d.outputs))
	for id := range d.outputs {
		result = append(result, id)
	}
	return result
}

// Node returns the FormulaNode for the given ID, or nil if not found.
func (d *DAG) Node(id string) *domain.FormulaNode {
	return d.nodes[id]
}

// IncomingEdges returns the edges whose target is the given node ID.
func (d *DAG) IncomingEdges(nodeID string) []domain.FormulaEdge {
	result := make([]domain.FormulaEdge, 0)
	for _, e := range d.edges {
		if e.Target == nodeID {
			result = append(result, e)
		}
	}
	return result
}
