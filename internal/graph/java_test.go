package graph

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePomDirectDependencies(t *testing.T) {
	g, err := parsePom(fixture(t, "maven/app/pom.xml"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	// Not counted: the dependencyManagement BOM, the profile's h2, the
	// plugin's asm and the commented-out dependency.
	wantGraph(t, g, 7, 6, 1, []string{
		"java:com.example:demo@0.0.1-SNAPSHOT -> java:org.springframework.boot:spring-boot-starter-web@", // managed version
		"java:com.example:demo@0.0.1-SNAPSHOT -> java:com.google.guava:guava@33.2.1-jre",                 // ${guava.version}
		"java:com.example:demo@0.0.1-SNAPSHOT -> java:org.projectlombok:lombok@${lombok.version}",        // unknown property kept
		"java:com.example:demo@0.0.1-SNAPSHOT -> java:com.example:demo-common@0.0.1-SNAPSHOT",            // ${project.*}
	}, []string{"java:com.example:demo@0.0.1-SNAPSHOT"})
	if s := g.GetNode("java:org.junit.jupiter:junit-jupiter@").GetMetadata("scope"); s != "test" {
		t.Errorf("junit scope = %q, want test", s)
	}
}

func TestParsePomLatin1(t *testing.T) {
	pom := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n<project><groupId>de.example</groupId><artifactId>b\xfcro</artifactId><version>1.0</version>" +
		"<dependencies><dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.2</version></dependency></dependencies></project>\n"
	g, err := parsePom([]byte(pom), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 2, 1, 1, []string{"java:de.example:büro@1.0 -> java:junit:junit@4.13.2"}, nil)
}

func TestParseMavenTGF(t *testing.T) {
	g, err := parseMavenTGF(fixture(t, "maven/app/tree.tgf"))
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, []string{
		"java:com.example:demo@0.0.1-SNAPSHOT -> java:org.springframework.boot:spring-boot-starter-web@3.3.0",
		"java:org.springframework:spring-web@6.1.8 -> java:org.springframework:spring-beans@6.1.8",
		"java:com.example:demo@0.0.1-SNAPSHOT -> java:io.netty:netty-transport-native-epoll@4.1.110.Final", // classifier label
	}, []string{"java:com.example:demo@0.0.1-SNAPSHOT"})
}

func TestParseMavenTGFMultiModule(t *testing.T) {
	second := "1931444790 com.example:demo-api:jar:0.0.1-SNAPSHOT\n" +
		"1891502635 com.fasterxml.jackson.core:jackson-core:jar:2.17.1:compile\n#\n1931444790 1891502635 compile\n"
	g, err := parseMavenTGF(append(fixture(t, "maven/app/tree.tgf"), second...))
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 11, 10, 2, []string{
		"java:com.example:demo-api@0.0.1-SNAPSHOT -> java:com.fasterxml.jackson.core:jackson-core@2.17.1",
	}, []string{"java:com.example:demo@0.0.1-SNAPSHOT", "java:com.example:demo-api@0.0.1-SNAPSHOT"})
}

func TestParseGradleLockfile(t *testing.T) {
	g, err := parseGradleLockfile(fixture(t, "gradle/app/gradle.lockfile"), "app")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, []string{
		"java:app@ -> java:com.google.guava:guava@33.2.1-jre",
		"java:app@ -> java:org.opentest4j:opentest4j@1.3.0",
	}, []string{"java:app@"})
	if c := g.GetNode("java:org.apiguardian:apiguardian-api@1.1.2").GetMetadata("configurations"); c != "testCompileClasspath" {
		t.Errorf("configurations = %q", c)
	}
}

func TestParseGradleDependencies(t *testing.T) {
	g, err := parseGradleDependencies(fixture(t, "gradle/app/dependencies.txt"), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 11, 12, 1, []string{
		"java:demo@ -> java:com.fasterxml.jackson.core:jackson-databind@2.17.1",                                // "g:a -> 2.17.1"
		"java:demo@ -> java:com.google.guava:guava@33.2.1-jre",                                                 // conflict arrow: resolved version
		"java:com.fasterxml.jackson.core:jackson-core@2.17.1 -> java:com.fasterxml.jackson:jackson-bom@2.17.1", // (*) still linked
		"java:demo@ -> java::core@",
		"java::core@ -> java:org.slf4j:slf4j-api@2.0.13",
	}, []string{"java:demo@"})
	if g.GetNode("java:com.google.guava:guava@33.0.0-jre") != nil {
		t.Error("the requested version of a conflict must not become a node")
	}
	for _, e := range g.Edges {
		if e.From == "java:org.springframework.boot:spring-boot-dependencies@3.3.0" {
			t.Errorf("(c) constraint became an edge: %s", e)
		}
	}
}

func TestGradleNodeMarkers(t *testing.T) {
	cases := []struct{ in, name, version string }{
		{"org.projectlombok:lombok:1.18.32 (n)", "org.projectlombok:lombok", "1.18.32"},
		{"com.example:missing:1.0 FAILED", "com.example:missing", "1.0"},
		{"org.slf4j:slf4j-api:{strictly 2.0.13} -> 2.0.13", "org.slf4j:slf4j-api", "2.0.13"},
	}
	for _, c := range cases {
		name, version, ok, err := gradleNode(c.in)
		if err != nil || !ok || name != c.name || version != c.version {
			t.Errorf("gradleNode(%q) = %q, %q, %v, %v", c.in, name, version, ok, err)
		}
	}
}

func TestParseJavaMalformed(t *testing.T) {
	if _, err := parsePom([]byte("<project><dependencies><dependency>"), "f"); err == nil {
		t.Error("pom.xml: want error for truncated XML")
	}
	if _, err := parsePom([]byte(`<?xml version="1.0" encoding="EBCDIC"?><project/>`), "f"); err == nil {
		t.Error("pom.xml: want error for an unsupported encoding")
	}
	if _, err := parseMavenTGF([]byte("1 com.example:demo:jar:1.0\n#\n1 2 compile\n")); err == nil {
		t.Error("tgf: want error for an edge to an unknown node")
	}
	if _, err := parseMavenTGF([]byte("1 not-a-maven-label\n")); err == nil {
		t.Error("tgf: want error for a bad label")
	}
	if _, err := parseGradleLockfile([]byte("com.google.guava:guava=compileClasspath\n"), "f"); err == nil {
		t.Error("gradle.lockfile: want error for coordinates without a version")
	}
	if _, err := parseGradleLockfile([]byte("<<<<<<< HEAD\n"), "f"); err == nil {
		t.Error("gradle.lockfile: want error for a line without '='")
	}
	if _, err := parseGradleDependencies([]byte("FAILURE: Build failed with an exception.\n"), "f"); err == nil {
		t.Error("gradle output: want error when there is no tree")
	}
	if _, err := parseGradleDependencies([]byte("+--- a:b:1.0\n|         \\--- c:d:1.0\n"), "f"); err == nil {
		t.Error("gradle output: want error for skipped indentation")
	}
}

// failRun fails the test if any external command runs.
func failRun(t *testing.T) func(string, string, ...string) ([]byte, error) {
	return func(_, name string, args ...string) ([]byte, error) {
		t.Fatalf("ran %s %v without Exec", name, args)
		return nil, nil
	}
}

func TestJavaExtractorParsesFilesWithoutExec(t *testing.T) {
	g, err := (&JavaExtractor{}).Extract(copyFixtureDir(t, "maven/app"), ExtractOptions{Run: failRun(t)})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 7, 6, 1, nil, nil)

	dir := copyFixtureDir(t, "gradle/app")
	g, err = (&JavaExtractor{}).Extract(dir, ExtractOptions{Run: failRun(t)})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, nil, []string{"java:" + filepath.Base(dir) + "@"})

	if err := os.Remove(filepath.Join(dir, "gradle.lockfile")); err != nil {
		t.Fatal(err)
	}
	if _, err := (&JavaExtractor{}).Extract(dir, ExtractOptions{Run: failRun(t)}); err == nil || !strings.Contains(err.Error(), "--exec") {
		t.Fatalf("err = %v, want a hint to use --exec", err)
	}
}

func TestJavaExtractorExecMaven(t *testing.T) {
	dir := copyFixtureDir(t, "maven/app")
	tgf := fixture(t, "maven/app/tree.tgf")
	var ran []string
	opts := ExtractOptions{Exec: true, Run: func(d, name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		for _, a := range args {
			if out, ok := strings.CutPrefix(a, "-DoutputFile="); ok {
				return nil, os.WriteFile(out, tgf, 0o644)
			}
		}
		return nil, errors.New("no -DoutputFile")
	}}
	g, err := (&JavaExtractor{}).Extract(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, nil, nil)
	if len(ran) != 1 || !strings.HasPrefix(ran[0], "mvn -q dependency:tree -DoutputType=tgf -DoutputFile=") {
		t.Errorf("ran %q", ran)
	}
}

func TestJavaExtractorExecGradleWrapperAndFallback(t *testing.T) {
	dir := copyFixtureDir(t, "gradle/app")
	wrapper := "gradlew"
	if runtimeIsWindows() {
		wrapper = "gradlew.bat"
	}
	if err := os.WriteFile(filepath.Join(dir, wrapper), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := fixture(t, "gradle/app/dependencies.txt")
	var ranName string
	opts := ExtractOptions{Exec: true, Run: func(_, name string, args ...string) ([]byte, error) {
		ranName = name
		return out, nil
	}}
	g, err := (&JavaExtractor{}).Extract(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 11, 12, 1, nil, []string{"java:demo@"})
	if filepath.Base(ranName) != wrapper || !filepath.IsAbs(ranName) {
		t.Errorf("ran %q, want the absolute path of the project's %s", ranName, wrapper)
	}

	var warnings []string
	opts = ExtractOptions{
		Exec: true,
		Run:  func(string, string, ...string) ([]byte, error) { return nil, errors.New("exit status 1") },
		Warn: func(m string) { warnings = append(warnings, m) },
	}
	g, err = (&JavaExtractor{}).Extract(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, nil, nil) // gradle.lockfile
	if len(warnings) != 1 || !strings.Contains(warnings[0], "gradle.lockfile") {
		t.Errorf("warnings = %q", warnings)
	}
}
