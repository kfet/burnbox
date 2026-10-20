package ui

import (
	"strings"
	"testing"
)

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

// The burnbox title on every page links home (relative, so it also works
// when mounted under a path prefix) without restyling the heading.
func TestTitleLinksHome(t *testing.T) {
	for name, page := range map[string][]byte{
		"index":  Index,
		"recipe": Recipe,
	} {
		s := string(page)
		if !strings.Contains(s, `title="burnbox home"><span class="b">burn</span>box</a>`) {
			t.Errorf("%s: title is not a home link", name)
		}
		if !strings.Contains(s, "h1 a { color:inherit; text-decoration:none; }") {
			t.Errorf("%s: home link would restyle the title", name)
		}
	}
	if !strings.Contains(string(Index), `<h1><a href="./"`) || !strings.Contains(string(Recipe), `<h1><a href="../"`) {
		t.Error("home link href wrong")
	}
}
