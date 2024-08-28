package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultitree(t *testing.T) {
	mt := multitree{}
	mt.newSubtreeFromString("a", nil)
	assert.Len(t, mt.trees, 1)
	tree0 := mt.trees[0]
	assert.NotNil(t, tree0.root)
	assert.Equal(t, tree0.root.data, "a")

	mt.pushString("b/c/d", nil)
	assert.Len(t, mt.trees, 2)
	tree1 := mt.trees[1]
	assert.NotNil(t, tree1.root)
	assert.Equal(t, tree1.root.data, "b")
	assert.Len(t, tree1.root.children, 1)
	tree1child0 := tree1.root.children[0]
	assert.Equal(t, tree1child0.data, "c")

	leafNodes := mt.leafNodes()
	assert.Len(t, leafNodes, 2)
	assert.Equal(t, leafNodes[0].data, "a")
	assert.Equal(t, leafNodes[1].data, "d")
}
