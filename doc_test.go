package burnbox

import (
	"os"
	"strings"
	"testing"
)

func TestVersionNonEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
	if Commit == "" {
		t.Fatal("Commit must not be empty")
	}
	if BuildDate == "" {
		t.Fatal("BuildDate must not be empty")
	}
}

// TestVersionMatchesFile guards against a hardcoded Version drifting
// from the VERSION file.
func TestVersionMatchesFile(t *testing.T) {
	b, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimSpace(string(b)); Version != want {
		t.Fatalf("Version = %q, VERSION file = %q", Version, want)
	}
}
