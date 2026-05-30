// Command lintfrontend performs cheap static checks over burnbox's
// embedded frontend assets. It is invoked by `make lint-frontend` and is
// not part of the go-test coverage surface.
//
// Checks:
//   - every referenced asset exists (index.html must reference burnbox.js)
//   - if `node` is on PATH, each .js file must pass `node --check`
//   - .html files must have balanced <script> open/close tags
//
// Usage: lintfrontend DIR [DIR...]
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lintfrontend DIR [DIR...]")
		os.Exit(2)
	}
	var problems int
	haveNode := lookNode()
	if !haveNode {
		fmt.Fprintln(os.Stderr, "note: node not found on PATH — skipping `node --check` JS parse pass")
	}
	for _, dir := range os.Args[1:] {
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read dir %s: %v\n", dir, err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			switch strings.ToLower(filepath.Ext(e.Name())) {
			case ".js":
				if haveNode {
					if out, err := exec.Command("node", "--check", path).CombinedOutput(); err != nil {
						problems++
						fmt.Fprintf(os.Stderr, "JS parse error in %s:\n%s\n", path, out)
					}
				}
			case ".html":
				if err := checkHTML(path); err != nil {
					problems++
					fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
				}
			}
		}
	}
	if problems > 0 {
		fmt.Fprintf(os.Stderr, "lintfrontend: %d problem(s)\n", problems)
		os.Exit(1)
	}
	fmt.Println("frontend assets ok")
}

func lookNode() bool {
	_, err := exec.LookPath("node")
	return err == nil
}

func checkHTML(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := strings.ToLower(string(b))
	open := strings.Count(s, "<script")
	close := strings.Count(s, "</script>")
	if open != close {
		return fmt.Errorf("unbalanced <script> tags: %d open, %d close", open, close)
	}
	return nil
}
