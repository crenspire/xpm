package runtimes

import (
	"context"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const adoptiumInfo = `{"available_releases":[8,11,17,21,25],"available_lts_releases":[8,11,17,21,25],"most_recent_feature_release":25,"most_recent_lts":25}`

func TestJavaReleaseNames(t *testing.T) {
	cases := map[string]string{"jdk-21.0.12.1+1": "21.0.12.1+1", "jdk8u422-b05": "8u422-b05", "jdk-17.0.9+9": "17.0.9+9"}
	for name, v := range cases {
		if got := javaVersionFromRelease(name); got != v {
			t.Errorf("%s -> %s, want %s", name, got, v)
		}
		if got := javaReleaseName(v); got != name {
			t.Errorf("%s -> %s, want %s", v, got, name)
		}
	}
}

func TestAdoptiumPlatform(t *testing.T) {
	cases := map[[2]string][2]string{
		{"darwin", "arm64"}:  {"mac", "aarch64"},
		{"linux", "amd64"}:   {"linux", "x64"},
		{"windows", "amd64"}: {"windows", "x64"},
	}
	for p, want := range cases {
		o, a, err := adoptiumPlatform(p[0], p[1])
		if err != nil || o != want[0] || a != want[1] {
			t.Errorf("%v: %s %s %v", p, o, a, err)
		}
	}
}

func TestJavaResolve(t *testing.T) {
	setHost(t, "darwin", "arm64")
	q := "architecture=aarch64&image_type=jdk&os=mac&vendor=eclipse"
	url, _ := newServer(t, map[string]route{
		"/v3/info/available_releases":       {body: []byte(adoptiumInfo)},
		"/v3/assets/latest/21/hotspot?" + q: {body: []byte(`[{"release_name":"jdk-21.0.12.1+1"}]`)},
		"/v3/assets/latest/25/hotspot?" + q: {body: []byte(`[{"release_name":"jdk-25.0.1+8"}]`)},
		"/v3/assets/latest/8/hotspot?" + q:  {body: []byte(`[{"release_name":"jdk8u422-b05"}]`)},
	})
	setVar(t, &adoptiumAPI, url)
	j := &JavaInstaller{}
	ctx := context.Background()
	for spec, want := range map[string]string{"21": "21.0.12.1+1", "8": "8u422-b05", "latest": "25.0.1+8", "lts": "25.0.1+8", "21.0.4+7": "21.0.4+7"} {
		if got, err := env.ResolveSpec(ctx, j, spec); err != nil || got != want {
			t.Errorf("%s: %q, %v; want %s", spec, got, err, want)
		}
	}
	if _, err := j.Resolve(ctx, "stable"); err == nil {
		t.Error("stable accepted")
	}
	if got, err := j.ListRemote(ctx); err != nil || strings.Join(got, ",") != "8,11,17,21,25" {
		t.Errorf("ListRemote = %v, %v", got, err)
	}
}

func TestJavaInstallMacHoistsContentsHome(t *testing.T) {
	skipWindows(t)
	setHost(t, "darwin", "arm64")
	top := "jdk-21.0.12.1+1/"
	var entries []tarEntry
	entries = append(entries, dir(top), reg(top+"Contents/Info.plist", "plist"))
	for _, b := range []string{"java", "javac", "jar", "keytool"} {
		entries = append(entries, reg(top+"Contents/Home/bin/"+b, b))
	}
	archive := tarGzBytes(t, entries)
	release := `{"binaries":[{"package":{"checksum":"` + shaBytes(archive) + `","link":"LINK","name":"OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.12.1_1.tar.gz"}}],"release_name":"jdk-21.0.12.1+1"}`
	q := "architecture=aarch64&heap_size=normal&image_type=jdk&jvm_impl=hotspot&os=mac"
	// The handler reads routes per request, so the release JSON can name
	// the server's own URL once it is known.
	routes := map[string]route{"/dl/OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.12.1_1.tar.gz": {body: archive}}
	url, seen := newServer(t, routes)
	routes["/v3/assets/release_name/eclipse/jdk-21.0.12.1+1?"+q] = route{body: []byte(strings.Replace(release, "LINK", url+"/dl/OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.12.1_1.tar.gz", 1))}
	setVar(t, &adoptiumAPI, url)
	dest := installInto(t, &JavaInstaller{}, "21.0.12.1+1")
	if _, err := readFile(dest, "Info.plist"); err == nil {
		t.Fatal("Contents/ leaked into the JDK root")
	}
	if got := (*seen)[0].URL.EscapedPath(); got != "/v3/assets/release_name/eclipse/jdk-21.0.12.1%2B1" {
		t.Fatalf("release path %s: '+' must be sent as %%2B", got)
	}
}

func TestJavaInstallJava8(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	top := "jdk8u422-b05/"
	entries := []tarEntry{dir(top), dir(top + "bin")}
	for _, b := range []string{"java", "javac", "jar", "keytool"} {
		entries = append(entries, reg(top+"bin/"+b, b))
	}
	archive := tarGzBytes(t, entries)
	file := "OpenJDK8U-jdk_x64_linux_hotspot_8u422b05.tar.gz"
	release := `{"binaries":[{"package":{"checksum":"` + shaBytes(archive) + `","link":"LINK","name":"` + file + `"}}],"release_name":"jdk8u422-b05"}`
	q := "architecture=x64&heap_size=normal&image_type=jdk&jvm_impl=hotspot&os=linux"
	routes := map[string]route{"/dl/" + file: {body: archive}}
	url, _ := newServer(t, routes)
	routes["/v3/assets/release_name/eclipse/jdk8u422-b05?"+q] = route{body: []byte(strings.Replace(release, "LINK", url+"/dl/"+file, 1))}
	setVar(t, &adoptiumAPI, url)
	dest := installInto(t, &JavaInstaller{}, "8u422-b05")
	for _, b := range (&JavaInstaller{}).BinaryPaths() {
		if got, err := readFile(dest, b); err != nil || got != strings.TrimPrefix(b, "bin/") {
			t.Errorf("%s: %q, %v", b, got, err)
		}
	}
}
