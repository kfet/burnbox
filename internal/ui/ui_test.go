package ui

import "testing"

func TestAssetsEmbedded(t *testing.T) {
	if len(Index) == 0 {
		t.Error("Index asset is empty")
	}
	if len(Script) == 0 {
		t.Error("Script asset is empty")
	}
	if len(Recipe) == 0 {
		t.Error("Recipe asset is empty")
	}
	if len(RecipeScript) == 0 {
		t.Error("RecipeScript asset is empty")
	}
}
