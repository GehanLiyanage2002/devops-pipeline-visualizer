package graph

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrEmptyNodeID is returned when attempting to add a node without an ID.
	ErrEmptyNodeID = errors.New("node ID must not be empty")
	// ErrInvalidEdgeEndpoints is returned when an edge is missing a source or target node ID.
	ErrInvalidEdgeEndpoints = errors.New("edge source and target must not be empty")
	// ErrEmptyGroupID is returned when attempting to add a group without an ID.
	ErrEmptyGroupID = errors.New("group ID must not be empty")
	// ErrOrphanedEdgeRef is returned when an edge references a source or target node that does not exist.
	ErrOrphanedEdgeRef = errors.New("orphaned edge reference")
	// ErrOrphanedGroupRef is returned when a node or group references a parent group that does not exist.
	ErrOrphanedGroupRef = errors.New("orphaned parent group reference")
	// ErrCycleDetected is returned when a cycle is found in the pipeline graph or group hierarchy.
	ErrCycleDetected = errors.New("cycle detected in pipeline DAG")
)

// NewPipelineGraph creates and initializes a new PipelineGraph with non-nil slices.
func NewPipelineGraph(id, name string) *PipelineGraph {
	return &PipelineGraph{
		ID:        id,
		Name:      name,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Nodes:     make([]Node, 0),
		Edges:     make([]Edge, 0),
		Groups:    make([]NodeGroup, 0),
	}
}

func (g *PipelineGraph) touch() {
	g.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func cloneMetadata(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// AddNode appends a node to the graph in a thread-safe manner.
// Note: Call DeduplicateNodes() to collapse duplicate nodes by ID and merge their metadata.
func (g *PipelineGraph) AddNode(n Node) error {
	if strings.TrimSpace(n.ID) == "" {
		return ErrEmptyNodeID
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if n.Status == "" {
		n.Status = StatusIdle
	}
	if n.ZoomLevel <= 0 {
		n.ZoomLevel = ZoomHighLevelStage
	}
	n.Metadata = cloneMetadata(n.Metadata)

	g.Nodes = append(g.Nodes, n)
	g.touch()
	return nil
}

// AddEdge adds a directed edge to the graph in a thread-safe manner.
// If the edge ID is empty, a deterministic ID is generated from source, target, and relation type.
// Exact duplicate edges (matching ID or Source+Target+RelationType) are updated in place.
func (g *PipelineGraph) AddEdge(e Edge) error {
	if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" {
		return ErrInvalidEdgeEndpoints
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if e.ID == "" {
		rel := string(e.RelationType)
		if rel == "" {
			rel = string(RelationTriggers)
			e.RelationType = RelationTriggers
		}
		e.ID = fmt.Sprintf("%s->%s:%s", e.Source, e.Target, rel)
	}

	for i := range g.Edges {
		existing := &g.Edges[i]
		if existing.ID == e.ID || (existing.Source == e.Source && existing.Target == e.Target && existing.RelationType == e.RelationType) {
			if e.Label != "" {
				existing.Label = e.Label
			}
			existing.Animated = existing.Animated || e.Animated
			existing.IsManual = existing.IsManual || e.IsManual
			g.touch()
			return nil
		}
	}

	g.Edges = append(g.Edges, e)
	g.touch()
	return nil
}

// AddGroup adds or updates a logical NodeGroup container in a thread-safe manner.
func (g *PipelineGraph) AddGroup(grp NodeGroup) error {
	if strings.TrimSpace(grp.ID) == "" {
		return ErrEmptyGroupID
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	for i := range g.Groups {
		if g.Groups[i].ID == grp.ID {
			if grp.Label != "" {
				g.Groups[i].Label = grp.Label
			}
			if grp.Provider != "" {
				g.Groups[i].Provider = grp.Provider
			}
			if grp.ResourceType != "" {
				g.Groups[i].ResourceType = grp.ResourceType
			}
			if grp.ParentGroupID != "" {
				g.Groups[i].ParentGroupID = grp.ParentGroupID
			}
			g.touch()
			return nil
		}
	}

	g.Groups = append(g.Groups, grp)
	g.touch()
	return nil
}

// DeduplicateNodes collapses nodes with identical IDs in a thread-safe manner,
// preserving first-seen order and merging non-empty attributes and Metadata maps.
func (g *PipelineGraph) DeduplicateNodes() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.Nodes) <= 1 {
		return
	}

	deduped := make([]Node, 0, len(g.Nodes))
	indexByID := make(map[string]int, len(g.Nodes))

	for _, n := range g.Nodes {
		if idx, exists := indexByID[n.ID]; exists {
			existing := &deduped[idx]
			if n.Label != "" {
				existing.Label = n.Label
			}
			if n.Stage != "" {
				existing.Stage = n.Stage
			}
			if n.Tool != "" {
				existing.Tool = n.Tool
			}
			if n.Status != "" && (existing.Status == "" || existing.Status == StatusIdle) {
				existing.Status = n.Status
			}
			if n.ZoomLevel > 0 {
				existing.ZoomLevel = n.ZoomLevel
			}
			if n.ParentGroupID != "" {
				existing.ParentGroupID = n.ParentGroupID
			}
			if n.Position.X != 0 || n.Position.Y != 0 {
				existing.Position = n.Position
			}
			existing.IsManual = existing.IsManual || n.IsManual

			if existing.Metadata == nil {
				existing.Metadata = make(map[string]interface{})
			}
			for k, v := range n.Metadata {
				existing.Metadata[k] = v
			}
		} else {
			cloned := n
			cloned.Metadata = cloneMetadata(n.Metadata)
			indexByID[n.ID] = len(deduped)
			deduped = append(deduped, cloned)
		}
	}

	g.Nodes = deduped
	g.touch()
}

// ValidateDAG verifies the integrity of the PipelineGraph in a thread-safe manner.
// It checks for:
//  1. Orphaned parent group references on nodes and groups
//  2. Cycles in group parent hierarchies
//  3. Orphaned source or target node references on edges
//  4. Self-loops and directed cycles among nodes and edges
func (g *PipelineGraph) ValidateDAG() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	groupMap := make(map[string]NodeGroup, len(g.Groups))
	for _, grp := range g.Groups {
		if strings.TrimSpace(grp.ID) == "" {
			return ErrEmptyGroupID
		}
		groupMap[grp.ID] = grp
	}

	// Validate group hierarchy references and detect group nesting cycles.
	for _, grp := range g.Groups {
		if grp.ParentGroupID == "" {
			continue
		}
		if _, ok := groupMap[grp.ParentGroupID]; !ok {
			return fmt.Errorf("%w: group %q references missing parent group %q", ErrOrphanedGroupRef, grp.ID, grp.ParentGroupID)
		}

		visitedGroups := map[string]bool{grp.ID: true}
		curr := grp.ParentGroupID
		for curr != "" {
			if visitedGroups[curr] {
				return fmt.Errorf("%w: group hierarchy cycle involving group %q", ErrCycleDetected, grp.ID)
			}
			visitedGroups[curr] = true
			parentGrp, ok := groupMap[curr]
			if !ok {
				return fmt.Errorf("%w: group %q references missing parent group %q", ErrOrphanedGroupRef, curr, grp.ParentGroupID)
			}
			curr = parentGrp.ParentGroupID
		}
	}

	nodeMap := make(map[string]Node, len(g.Nodes))
	nodeOrder := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			return ErrEmptyNodeID
		}
		if n.ParentGroupID != "" {
			if _, ok := groupMap[n.ParentGroupID]; !ok {
				return fmt.Errorf("%w: node %q references missing parent group %q", ErrOrphanedGroupRef, n.ID, n.ParentGroupID)
			}
		}
		if _, exists := nodeMap[n.ID]; !exists {
			nodeOrder = append(nodeOrder, n.ID)
		}
		nodeMap[n.ID] = n
	}

	adj := make(map[string][]string, len(nodeMap))
	for _, e := range g.Edges {
		if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" {
			return ErrInvalidEdgeEndpoints
		}
		if _, ok := nodeMap[e.Source]; !ok {
			return fmt.Errorf("%w: edge %q references missing source node %q", ErrOrphanedEdgeRef, e.ID, e.Source)
		}
		if _, ok := nodeMap[e.Target]; !ok {
			return fmt.Errorf("%w: edge %q references missing target node %q", ErrOrphanedEdgeRef, e.ID, e.Target)
		}
		if e.Source == e.Target {
			return fmt.Errorf("%w: self-loop on node %q", ErrCycleDetected, e.Source)
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
	}

	// 3-color DFS cycle detection:
	// 0 = unvisited, 1 = visiting (on current recursion stack), 2 = visited (fully explored)
	const (
		stateUnvisited = 0
		stateVisiting  = 1
		stateVisited   = 2
	)
	state := make(map[string]int, len(nodeMap))
	stack := make([]string, 0, len(nodeMap))

	var dfs func(u string) error
	dfs = func(u string) error {
		state[u] = stateVisiting
		stack = append(stack, u)

		for _, v := range adj[u] {
			switch state[v] {
			case stateVisiting:
				// Reconstruct cycle path from stack
				cyclePath := []string{v}
				for i := len(stack) - 1; i >= 0; i-- {
					cyclePath = append(cyclePath, stack[i])
					if stack[i] == v {
						break
					}
				}
				// Reverse cyclePath to show forward direction
				for l, r := 0, len(cyclePath)-1; l < r; l, r = l+1, r-1 {
					cyclePath[l], cyclePath[r] = cyclePath[r], cyclePath[l]
				}
				return fmt.Errorf("%w: %s", ErrCycleDetected, strings.Join(cyclePath, " -> "))
			case stateUnvisited:
				if err := dfs(v); err != nil {
					return err
				}
			}
		}

		stack = stack[:len(stack)-1]
		state[u] = stateVisited
		return nil
	}

	for _, id := range nodeOrder {
		if state[id] == stateUnvisited {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}

	return nil
}
