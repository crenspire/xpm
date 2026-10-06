package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes data to dir/name, creating parent directories.
func writeFile(t *testing.T, dir, name, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	npmLock  = "{\"lockfileVersion\":3,\"packages\":{\"\":{},\"node_modules/a\":{\"version\":\"1.0.0\"}}}\n"
	yarnLock = "# yarn lockfile v1\n\na@^1.0.0:\n  version \"1.0.0\"\n"
	goSum    = "example.com/m v1.0.0 h1:abc=\nexample.com/m v1.0.0/go.mod h1:def=\n"
)

func TestGenerateKeysByPathSoNodeLockfilesDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", npmLock)
	writeFile(t, dir, "yarn.lock", yarnLock)

	u, warnings, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if u.Count() != 2 {
		t.Fatalf("Count = %d, want 2 (package-lock.json and yarn.lock): %+v", u.Count(), u.Locks)
	}
	if got := u.Locks["package-lock.json"]; got == nil || got.Manager != "npm" || got.Packages != 1 {
		t.Errorf("package-lock.json entry = %+v", got)
	}
	if got := u.Locks["yarn.lock"]; got == nil || got.Manager != "yarn" || got.Packages != 1 {
		t.Errorf("yarn.lock entry = %+v", got)
	}
}

func TestMarshalIsExactAndHasNoTimestamps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "yarn.lock", yarnLock)
	writeFile(t, dir, "go.sum", goSum)
	u, _, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 2\n" +
		"locks:\n" +
		"  go.sum:\n" +
		"    ecosystem: go\n" +
		"    manager: go\n" +
		"    file: go.sum\n" +
		"    hash: " + hashBytes([]byte(goSum)) + "\n" +
		"    packages: 1\n" +
		"  yarn.lock:\n" +
		"    ecosystem: node\n" +
		"    manager: yarn\n" +
		"    file: yarn.lock\n" +
		"    hash: " + hashBytes([]byte(yarnLock)) + "\n" +
		"    packages: 1\n"
	if string(data) != want {
		t.Fatalf("Marshal =\n%s\nwant\n%s", data, want)
	}
}

func TestWriteUnifiedLockSkipsIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", npmLock)

	u, _, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := WriteUnifiedLock(dir, u)
	if err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v, want true nil", changed, err)
	}
	first, err := os.ReadFile(filepath.Join(dir, LockfileName))
	if err != nil {
		t.Fatal(err)
	}

	u2, _, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err = WriteUnifiedLock(dir, u2)
	if err != nil || changed {
		t.Fatalf("second write: changed=%v err=%v, want false nil", changed, err)
	}
	second, err := os.ReadFile(filepath.Join(dir, LockfileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("regenerating an unchanged project changed xpm-lock.yaml:\n%s\n---\n%s", first, second)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestWriteUnifiedLockRewritesWhenContentDiffers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, LockfileName, "version: 1\n")
	writeFile(t, dir, "go.sum", goSum)
	u, _, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := WriteUnifiedLock(dir, u)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v, want true nil", changed, err)
	}
	got, err := ReadUnifiedLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion || got.Locks["go.sum"] == nil {
		t.Fatalf("ReadUnifiedLock = %+v", got)
	}
}

func TestVerifyReportsEveryStatus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", npmLock)
	writeFile(t, dir, "yarn.lock", yarnLock)
	writeFile(t, dir, "go.sum", goSum)
	u, _, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteUnifiedLock(dir, u); err != nil {
		t.Fatal(err)
	}

	// yarn.lock changes, go.sum disappears, Cargo.lock appears.
	writeFile(t, dir, "yarn.lock", yarnLock+"\nb@^2.0.0:\n  version \"2.0.0\"\n")
	if err := os.Remove(filepath.Join(dir, "go.sum")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "Cargo.lock", "version = 3\n")

	results, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]VerificationStatus{}
	var order []string
	for _, r := range results {
		got[r.Key] = r.Status
		order = append(order, r.Key)
	}
	want := map[string]VerificationStatus{
		"Cargo.lock":        StatusAdded,
		"go.sum":            StatusMissing,
		"package-lock.json": StatusUnchanged,
		"yarn.lock":         StatusChanged,
	}
	if len(got) != len(want) {
		t.Fatalf("results = %+v, want %v", results, want)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: status %v, want %v", k, got[k], w)
		}
	}
	if strings.Join(order, ",") != "Cargo.lock,go.sum,package-lock.json,yarn.lock" {
		t.Errorf("order = %v, want sorted by path", order)
	}
	if VerificationPassed(results) {
		t.Error("VerificationPassed = true, want false")
	}
}

func TestVerifyReadsVersion1Files(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", npmLock)
	writeFile(t, dir, "yarn.lock", yarnLock)
	// xpm v1 keyed by ecosystem; yarn.lock was silently dropped by the collision.
	writeFile(t, dir, LockfileName, "version: 1\n"+
		"generatedAt: 2026-01-02T03:04:05Z\n"+
		"locks:\n"+
		"    node:\n"+
		"        ecosystem: node\n"+
		"        manager: npm\n"+
		"        file: package-lock.json\n"+
		"        hash: "+hashBytes([]byte(npmLock))+"\n"+
		"        modified: 2026-01-02T03:04:05Z\n"+
		"        packages: 1\n")

	results, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2", results)
	}
	if results[0].Key != "package-lock.json" || results[0].Status != StatusUnchanged {
		t.Errorf("results[0] = %+v, want package-lock.json unchanged", results[0])
	}
	if results[1].Key != "yarn.lock" || results[1].Status != StatusAdded {
		t.Errorf("results[1] = %+v, want yarn.lock added", results[1])
	}
}

func TestVerifyNeverOpensPathsOutsideTheProject(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "project")
	writeFile(t, parent, "secret.lock", "s3cret")
	writeFile(t, dir, LockfileName, "version: 2\n"+
		"locks:\n"+
		"  ../secret.lock:\n"+
		"    file: ../secret.lock\n"+
		"    hash: "+hashBytes([]byte("s3cret"))+"\n"+
		"  /etc/hosts:\n"+
		"    file: /etc/hosts\n"+
		"    hash: 00\n")

	results, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2", results)
	}
	for _, r := range results {
		if r.Status != StatusError || r.Error == nil {
			t.Errorf("%s: status %v err %v, want StatusError with a reason", r.Key, r.Status, r.Error)
		}
		if r.ActualHash != "" {
			t.Errorf("%s: ActualHash = %q; the file must never be read", r.Key, r.ActualHash)
		}
	}
}

func TestReadUnifiedLockRejectsNewerVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, LockfileName, "version: 3\nlocks: {}\n")
	if _, err := ReadUnifiedLock(dir); err == nil || !strings.Contains(err.Error(), "version 3") {
		t.Fatalf("ReadUnifiedLock(version 3) err = %v, want a version error", err)
	}
}

func TestGenerateWarnsButRecordsUncountableLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", "{ not json")
	u, warnings, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "package-lock.json") {
		t.Fatalf("warnings = %v, want one about package-lock.json", warnings)
	}
	if info := u.Locks["package-lock.json"]; info == nil || info.Packages != 0 || info.Hash == "" {
		t.Fatalf("entry = %+v, want recorded with hash and 0 packages", info)
	}
}
