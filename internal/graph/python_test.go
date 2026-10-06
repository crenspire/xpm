package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePoetryLock(t *testing.T) {
	g, err := parsePoetryLock(fixture(t, "python/poetry/poetry.lock"), fixture(t, "python/poetry/pyproject.toml"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	// 14 packages + the project; 12 package edges + 3 from the project.
	wantGraph(t, g, 15, 15, 1, []string{
		"python:pydantic@2.7.4 -> python:typing-extensions@4.12.2", // written "typing_extensions"
		"python:pydantic-core@2.18.4 -> python:typing-extensions@4.12.2",
		"python:pytest@8.2.2 -> python:colorama@0.4.6", // inline-table dependency
		"python:requests@2.32.3 -> python:urllib3@2.2.2",
		"python:py-app@0.1.0 -> python:pydantic@2.7.4", // declared "Pydantic"
		"python:py-app@0.1.0 -> python:pytest@8.2.2",   // dev group
	}, []string{"python:py-app@0.1.0"})
}

func TestParsePyprojectPEP621IsFlat(t *testing.T) {
	info, err := parsePyproject(fixture(t, "python/pep621/pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	g := flatPythonGraph(info, "fallback")
	wantGraph(t, g, 7, 6, 1, []string{
		"python:pep621-app@0.2.0 -> python:httpx@",  // extras stripped
		"python:pep621-app@0.2.0 -> python:Rich@",   // "Rich ~= 13.7"
		"python:pep621-app@0.2.0 -> python:tomli@",  // environment marker
		"python:pep621-app@0.2.0 -> python:click@",  // optional-dependencies
		"python:pep621-app@0.2.0 -> python:ruff@",   // dependency-groups
		"python:pep621-app@0.2.0 -> python:pytest@", // include-group table skipped
	}, []string{"python:pep621-app@0.2.0"})
}

func TestParseRequirements(t *testing.T) {
	g, err := parseRequirements(fixture(t, "python/requirements/requirements.txt"), "reqs")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 5, 4, 1, []string{
		"python:reqs@ -> python:Django@5.0.6",
		"python:reqs@ -> python:djangorestframework@", // a range is not a version
		"python:reqs@ -> python:psycopg@3.1.19",       // extras and marker
		"python:reqs@ -> python:gunicorn@22.0.0",      // line continuation + --hash
	}, []string{"python:reqs@"})
}

func TestParsePythonMalformed(t *testing.T) {
	if _, err := parsePoetryLock([]byte("[[package]]\nname = \"x\"\nversion = \n"), nil, "f"); err == nil {
		t.Error("poetry.lock: want error for invalid TOML")
	}
	if _, err := parsePoetryLock([]byte("[[package]]\nname = \"x\"\n"), nil, "f"); err == nil {
		t.Error("poetry.lock: want error for a package without version")
	}
	if _, err := parsePoetryLock(fixture(t, "python/poetry/poetry.lock"), []byte("[tool.poetry\n"), "f"); err == nil {
		t.Error("pyproject.toml: want error for invalid TOML")
	}
	conflict := "requests==2.32.3\n<<<<<<< HEAD\nflask==3.0.3\n=======\nflask==3.0.2\n>>>>>>> feature\n"
	if _, err := parseRequirements([]byte(conflict), "f"); err == nil {
		t.Error("requirements.txt: want error for merge-conflict markers")
	}
}

func TestPythonExtractorMalformedPoetryLockIsNotSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "poetry.lock"), []byte("[[package]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&PythonExtractor{}).Extract(dir, ExtractOptions{}); err == nil {
		t.Fatal("a broken poetry.lock must be reported, not replaced by requirements.txt")
	}
}

func TestPythonExtractorFallbacks(t *testing.T) {
	g, err := (&PythonExtractor{}).Extract(copyFixtureDir(t, "python/poetry"), ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 15, 15, 1, nil, []string{"python:py-app@0.1.0"})

	g, err = (&PythonExtractor{}).Extract(copyFixtureDir(t, "python/pep621"), ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 7, 6, 1, nil, []string{"python:pep621-app@0.2.0"})

	dir := copyFixtureDir(t, "python/requirements")
	g, err = (&PythonExtractor{}).Extract(dir, ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 5, 4, 1, nil, []string{"python:" + filepath.Base(dir) + "@"})
}
