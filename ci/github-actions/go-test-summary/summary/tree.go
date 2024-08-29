package summary

import "strings"

type (
	// holds a split test path object and the original event which generated it
	pathWithEvent struct {
		path []string
		ev   *event
	}
	// convenience wrapper around a slice of trees for handling multiple root nodes
	multitree struct {
		trees []*tree
	}
	// tree to hold test structures of arbitrary cardinality
	/*
		                                  TestA
										 /     \
										/       \
		                            SubTestA  SubTestB
								   /   |    \
								  /    |     \
								 /     |      \
							   Case1  Case2  Case3
	*/
	tree struct {
		root *node
	}
	// node which holds its path part as data and the event that created it as well.
	// creating event is only relevant for leaf nodes in order to report their full
	// test name.
	node struct {
		data     string
		ev       *event
		children []*node
	}
)

func (m *multitree) leafNodes() []*node {
	var result []*node
	for _, subtree := range m.trees {
		result = append(result, subtree.leafNodes()...)
	}

	return result
}

func (m *multitree) pushString(s string, ev *event) {
	parts := strings.Split(s, "/")
	if len(parts) == 0 {
		return
	}

	rootPart := parts[0]
	var owningTree *tree
	for _, subtree := range m.trees {
		if subtree.root.data == rootPart {
			owningTree = subtree
			break
		}
	}
	if owningTree == nil {
		owningTree = m.newSubtreeFromString(rootPart, ev)
	}

	owningTree.insert(&pathWithEvent{
		path: parts[1:],
		ev:   ev,
	})
}

// newSubtreeFromString takes a string test path and splits it into constituent
// parts, creating a new subtree from the root of the test path.
func (m *multitree) newSubtreeFromString(s string, ev *event) *tree {
	newTree := &tree{
		root: &node{
			data:     s,
			ev:       ev,
			children: []*node{},
		},
	}
	m.trees = append(m.trees, newTree)
	return newTree
}

func (t *tree) leafNodes() []*node {
	if t.root == nil {
		return nil
	}

	if len(t.root.children) == 0 {
		return []*node{t.root}
	}

	result := []*node{}
	for _, child := range t.root.children {
		result = append(result, t.leafNodesRec(child)...)
	}

	return result
}

func (t *tree) leafNodesRec(n *node) []*node {
	// check if we have reached a leaf node (node with no children)
	if len(n.children) == 0 {
		return []*node{n}
	}

	result := []*node{}

	// since this node has children, keep recursing and aggregating any
	// leaf nodes we find.
	for _, child := range n.children {
		result = append(result, t.leafNodesRec(child)...)
	}

	return result
}

func (t *tree) insert(p *pathWithEvent) {
	if len(p.path) == 0 {
		return
	}
	if t.root == nil {
		panic("root should never be nil; construct tree with newSubtreeFromString")
	}
	t.insertRec(t.root, p)
}

func (t *tree) insertRec(n *node, p *pathWithEvent) *node {
	// we have reached the leaf; return the node we are at
	if len(p.path) == 0 {
		return n
	}
	if n == nil {
		panic("nil node at " + p.path[0])
	}

	var child *node
	// check if we have an existing subtree to go down
	for _, childNode := range n.children {
		if childNode.data == p.path[0] {
			child = childNode
		}
	}

	if child == nil {
		// create a new child node
		child = &node{
			data: p.path[0],
			ev:   p.ev,
		}
		n.children = append(n.children, child)
	}

	return t.insertRec(child, &pathWithEvent{
		path: p.path[1:],
		ev:   p.ev,
	})
}
