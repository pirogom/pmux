package server

import (
	"testing"
)

func TestInsertLayoutNode_RatioPreservation(t *testing.T) {
	// 1. Initial State: 1 pane (A)
	var root *LayoutNode = &LayoutNode{ID: "A"}

	// 2. Split A vertically -> [A, B]
	root = insertLayoutNode(root, "A", "B", SplitVertical)

	if root.Direction != SplitVertical {
		t.Fatalf("expected direction vertical, got %s", root.Direction)
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(root.Children))
	}
	if root.Ratio != 1.0 {
		t.Errorf("expected root ratio 1.0, got %f", root.Ratio)
	}
	if root.Children[0].ID != "A" || root.Children[1].ID != "B" {
		t.Fatalf("expected children A and B, got %s and %s", root.Children[0].ID, root.Children[1].ID)
	}

	// 3. Split B vertically -> [A, [B, C]]
	root = insertLayoutNode(root, "B", "C", SplitVertical)

	if len(root.Children) != 2 {
		t.Fatalf("expected root to have 2 children, got %d", len(root.Children))
	}

	leftNode := root.Children[0]
	rightBranch := root.Children[1]

	if leftNode.ID != "A" {
		t.Errorf("expected left node to be A, got %s", leftNode.ID)
	}
	if rightBranch.ID != "" {
		t.Errorf("expected right branch node to be a container (empty ID), got %s", rightBranch.ID)
	}

	// The right branch node must retain the ratio of B in the root container (1.0),
	// so that root treats leftNode (flex 1) and rightBranch (flex 1) equally (50%:50%).
	if rightBranch.Ratio != 1.0 {
		t.Errorf("expected right branch node ratio to be 1.0, got %f", rightBranch.Ratio)
	}

	if len(rightBranch.Children) != 2 {
		t.Fatalf("expected right branch to have 2 children, got %d", len(rightBranch.Children))
	}
	if rightBranch.Children[0].ID != "B" || rightBranch.Children[1].ID != "C" {
		t.Errorf("expected right branch children to be B and C, got %s and %s", rightBranch.Children[0].ID, rightBranch.Children[1].ID)
	}
}

func TestInsertLayoutNode_ResizedRatioPreservation(t *testing.T) {
	// Root with resized A (0.6) and B (0.4)
	root := &LayoutNode{
		Direction: SplitVertical,
		Ratio:     1.0,
		Children: []*LayoutNode{
			{ID: "A", Ratio: 0.6},
			{ID: "B", Ratio: 0.4},
		},
	}

	// Split B horizontally -> [A(0.6), Branch[B(0), C(0)](0.4)]
	root = insertLayoutNode(root, "B", "C", SplitHorizontal)

	leftNode := root.Children[0]
	rightBranch := root.Children[1]

	if leftNode.Ratio != 0.6 {
		t.Errorf("expected left node A ratio to remain 0.6, got %f", leftNode.Ratio)
	}
	if rightBranch.Ratio != 0.4 {
		t.Errorf("expected right branch container to inherit B's ratio 0.4, got %f", rightBranch.Ratio)
	}
	if rightBranch.Direction != SplitHorizontal {
		t.Errorf("expected right branch direction to be horizontal, got %s", rightBranch.Direction)
	}
}
