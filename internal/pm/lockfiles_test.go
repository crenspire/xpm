package pm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectLockFilesIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "bun.lock", "yarn.lock", "package-lock.json", "poetry.lock", "requirements.txt")
	for i := 0; i < 50; i++ { // map iteration order would vary across calls
		got := DetectLockFiles(dir)
		if fmt.Sprint(got[EcosystemNode]) != "[npm yarn bun]" || fmt.Sprint(got[EcosystemPython]) != "[poetry pip]" {
			t.Fatalf("call %d: %v", i, got)
		}
	}
}

func TestBunLockFormatsCountOnce(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "bun.lock", "bun.lockb")
	if got := DetectLockFiles(dir)[EcosystemNode]; fmt.Sprint(got) != "[bun]" {
		t.Fatalf("got %v, want [bun]", got)
	}
}

func TestPipfileWithoutLockImpliesPipenv(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "Pipfile")
	if got := DetectLockFiles(dir)[EcosystemPython]; fmt.Sprint(got) != "[pipenv]" {
		t.Fatalf("got %v, want [pipenv]", got)
	}
}

func TestProjectManagersIncludesJavaBuildFiles(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "build.gradle.kts", "pnpm-lock.yaml")
	got := ProjectManagers(dir)
	java := got[EcosystemJava]
	if len(java) != 1 || java[0].Manager != Gradle || java[0].Name != "build.gradle.kts" {
		t.Fatalf("java = %+v, want gradle via build.gradle.kts", java)
	}
	if node := got[EcosystemNode]; len(node) != 1 || node[0].Manager != Pnpm || node[0].Name != "pnpm-lock.yaml" {
		t.Fatalf("node = %+v", node)
	}
	if _, ok := DetectLockFiles(dir)[EcosystemJava]; ok {
		t.Fatal("DetectLockFiles must stay node/python only (workspace installs rely on it)")
	}
}

func TestJavaEcosystem(t *testing.T) {
	if EcosystemForManager(Gradle) != EcosystemJava || EcosystemForManager(Maven) != EcosystemJava {
		t.Fatal("maven and gradle belong to the java ecosystem")
	}
	if fmt.Sprint(ManagersInEcosystem(EcosystemJava)) != "[maven gradle]" {
		t.Fatal(ManagersInEcosystem(EcosystemJava))
	}
}

func TestDetectLockFilesRefusesTraversal(t *testing.T) {
	if got := DetectLockFiles("../.."); len(got) != 0 {
		t.Fatalf("got %v for a path with ..", got)
	}
}
