package ssht_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHelpDocumentsRaycastOption(t *testing.T) {
	cmd := exec.Command("sh", "install.sh", "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh --help failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--raycast") {
		t.Fatalf("help should document --raycast option:\n%s", out)
	}
}

func TestInstallRaycastRequiresNPM(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoBuildStub(t, binDir)
	installDir := filepath.Join(tmp, "install")
	cmd := exec.Command("sh", "install.sh", "--raycast")
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin:/usr/sbin:/sbin",
		"INSTALL_DIR="+installDir,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install.sh --raycast should fail when npm is unavailable:\n%s", out)
	}
	text := string(out)
	if !strings.Contains(text, "npm is required") {
		t.Fatalf("missing npm dependency message:\n%s", text)
	}
	if !strings.Contains(text, filepath.Join(installDir, "ssht")) {
		t.Fatalf("CLI should be installed before Raycast dependency check:\n%s", text)
	}
}

func TestInstallRaycastUsesLocalRaycastCLIFromNPM(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoBuildStub(t, binDir)
	logPath := filepath.Join(tmp, "npm.log")
	npmStub := filepath.Join(binDir, "npm")
	npmScript := "#!/bin/sh\necho \"npm $*\" >>\"$SSHT_STUB_LOG\"\nif [ \"$1\" = \"run\" ] && [ \"$2\" = \"dev\" ]; then\n  echo 'ready  - built extension successfully'\n  sleep 30\nfi\nexit 0\n"
	if err := os.WriteFile(npmStub, []byte(npmScript), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "install.sh", "--raycast")
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin:/usr/sbin:/sbin",
		"INSTALL_DIR="+filepath.Join(tmp, "install"),
		"SSHT_STUB_LOG="+logPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh --raycast should not require global ray CLI: %v\n%s", err, out)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	for _, want := range []string{"npm ci", "npm run dev -- --non-interactive"} {
		if !strings.Contains(log, want) {
			t.Fatalf("missing %q in npm log:\n%s", want, log)
		}
	}
	if !strings.Contains(string(out), "Registered Raycast extension") {
		t.Fatalf("missing registration success message:\n%s", out)
	}
}

func writeGoBuildStub(t *testing.T, binDir string) {
	t.Helper()
	goStub := filepath.Join(binDir, "go")
	script := "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then\n    shift\n    mkdir -p \"$(dirname \"$1\")\"\n    printf '#!/bin/sh\\n' >\"$1\"\n    chmod +x \"$1\"\n    exit 0\n  fi\n  shift\ndone\nexit 1\n"
	if err := os.WriteFile(goStub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
