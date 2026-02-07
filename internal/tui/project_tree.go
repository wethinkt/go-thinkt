package tui

import (
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// treeNodeKind distinguishes directory headers from leaf projects.
type treeNodeKind int

const (
	treeNodeDir  treeNodeKind = iota // collapsible directory group
	treeNodeLeaf                     // selectable project
)

// treeNode is the internal tree representation before flattening.
type treeNode struct {
	label    string              // display segment: compacted path for dirs, project name for leaves
	fullPath string              // full path to this point
	kind     treeNodeKind
	expanded bool
	children []*treeNode
	project  *aggregatedProject // non-nil only for leaves
}

// maxDate returns the most recent LastModified among all descendant leaves.
func (n *treeNode) maxDate() time.Time {
	if n.kind == treeNodeLeaf && n.project != nil {
		return n.project.LastModified
	}
	var max time.Time
	for _, c := range n.children {
		if d := c.maxDate(); d.After(max) {
			max = d
		}
	}
	return max
}

// treeItem is a single row in the flattened list, implementing list.Item.
type treeItem struct {
	node   *treeNode
	depth  int
	isLast []bool // isLast[i] = whether the ancestor at depth i was the last sibling
}

func (t treeItem) Title() string {
	return t.node.label
}

func (t treeItem) Description() string { return "" }

func (t treeItem) FilterValue() string {
	if t.node.kind == treeNodeLeaf && t.node.project != nil {
		return t.node.project.Path + " " + t.node.project.Name
	}
	return t.node.fullPath + " " + t.node.label
}

// splitPathSegments splits a shortened path like "~/dev/foo" into segments.
// Returns ["~", "dev", "foo"].
func splitPathSegments(path string) []string {
	if path == "" {
		return []string{"~"}
	}
	// Clean the path
	path = filepath.Clean(path)
	parts := strings.Split(path, string(filepath.Separator))
	// Filter empty segments
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{"~"}
	}
	return result
}

// buildProjectTree builds a tree from aggregated projects.
// Projects are grouped by their path segments into a trie-like structure.
func buildProjectTree(projects []aggregatedProject) []*treeNode {
	root := &treeNode{kind: treeNodeDir, expanded: true}

	for i := range projects {
		p := &projects[i]
		segments := splitPathSegments(p.Name) // Name is already shortened (~/dev/foo)

		// Walk down the tree, creating intermediate directory nodes
		current := root
		for si, seg := range segments {
			isLastSegment := si == len(segments)-1

			if isLastSegment {
				// This is the leaf: the project itself
				leaf := &treeNode{
					label:    seg,
					fullPath: p.Name,
					kind:     treeNodeLeaf,
					project:  p,
				}
				current.children = append(current.children, leaf)
			} else {
				// Find or create intermediate directory
				var found *treeNode
				partialPath := strings.Join(segments[:si+1], "/")
				for _, c := range current.children {
					if c.kind == treeNodeDir && c.label == seg && c.fullPath == partialPath {
						found = c
						break
					}
				}
				if found == nil {
					found = &treeNode{
						label:    seg,
						fullPath: partialPath,
						kind:     treeNodeDir,
						expanded: true,
					}
					current.children = append(current.children, found)
				}
				current = found
			}
		}
	}

	return root.children
}

// compactTree merges single-child directory chains.
// If a directory node has exactly one child and that child is also a directory,
// they are merged: parent.label = "parent/child", parent.children = child.children.
func compactTree(nodes []*treeNode) {
	for _, n := range nodes {
		if n.kind != treeNodeDir {
			continue
		}
		// Compact: merge single-dir-child chains
		for len(n.children) == 1 && n.children[0].kind == treeNodeDir {
			child := n.children[0]
			n.label = n.label + "/" + child.label
			n.fullPath = child.fullPath
			n.children = child.children
			n.expanded = child.expanded
		}
		compactTree(n.children)
	}
}

// sortTree recursively sorts children within each directory node.
func sortTree(nodes []*treeNode, field sortField, dir sortDir) {
	sort.SliceStable(nodes, func(i, j int) bool {
		// Dirs before leaves
		if nodes[i].kind != nodes[j].kind {
			return nodes[i].kind == treeNodeDir
		}

		var less bool
		switch field {
		case sortByName:
			less = strings.ToLower(nodes[i].label) < strings.ToLower(nodes[j].label)
		default: // sortByDate
			less = nodes[i].maxDate().Before(nodes[j].maxDate())
		}
		if dir == sortDesc {
			return !less
		}
		return less
	})

	for _, n := range nodes {
		if n.kind == treeNodeDir && len(n.children) > 0 {
			sortTree(n.children, field, dir)
		}
	}
}

// flattenTree performs a pre-order walk, emitting only expanded subtrees.
func flattenTree(nodes []*treeNode) []treeItem {
	var items []treeItem
	flattenNodes(nodes, 0, nil, &items)
	return items
}

func flattenNodes(nodes []*treeNode, depth int, ancestorIsLast []bool, items *[]treeItem) {
	for i, n := range nodes {
		isLast := make([]bool, depth+1)
		copy(isLast, ancestorIsLast)
		isLast[depth] = (i == len(nodes)-1)

		*items = append(*items, treeItem{
			node:   n,
			depth:  depth,
			isLast: isLast,
		})

		if n.kind == treeNodeDir && n.expanded {
			flattenNodes(n.children, depth+1, isLast, items)
		}
	}
}

// restoreExpandState applies saved expand state to tree nodes.
func restoreExpandState(nodes []*treeNode, state map[string]bool) {
	for _, n := range nodes {
		if n.kind == treeNodeDir {
			if expanded, ok := state[n.fullPath]; ok {
				n.expanded = expanded
			}
			restoreExpandState(n.children, state)
		}
	}
}

// saveExpandState captures current expand state from tree nodes.
func saveExpandState(nodes []*treeNode, state map[string]bool) {
	for _, n := range nodes {
		if n.kind == treeNodeDir {
			state[n.fullPath] = n.expanded
			saveExpandState(n.children, state)
		}
	}
}

// countLeaves returns the number of leaf nodes in a tree.
func countLeaves(nodes []*treeNode) int {
	count := 0
	for _, n := range nodes {
		if n.kind == treeNodeLeaf {
			count++
		} else {
			count += countLeaves(n.children)
		}
	}
	return count
}

// treePrefix builds the tree connector prefix string for a flat item.
// Returns strings like "│   ├── " or "    └── ".
func treePrefix(depth int, isLast []bool) string {
	if depth == 0 {
		return ""
	}

	var b strings.Builder
	// Ancestor continuation lines
	for level := 0; level < depth-1; level++ {
		if isLast[level] {
			b.WriteString("    ")
		} else {
			b.WriteString("│   ")
		}
	}
	// This node's connector
	if isLast[depth-1] {
		b.WriteString("└── ")
	} else {
		b.WriteString("├── ")
	}
	return b.String()
}
