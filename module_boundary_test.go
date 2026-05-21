package ssht_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMainModuleSkipsRaycastTree(t *testing.T) {
	if _, err := os.Stat("raycast/go.mod"); err != nil {
		t.Fatalf("raycast/go.mod must exist so go ./... skips Raycast dependencies: %v", err)
	}

	out, err := exec.Command("go", "list", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./... failed: %v\n%s", err, out)
	}

	for _, pkg := range strings.Fields(string(out)) {
		if strings.Contains(pkg, "/raycast/") {
			t.Fatalf("main module should not enumerate Raycast packages, got %q", pkg)
		}
	}
}
