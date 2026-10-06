package doctor

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestHeaderNamesXpm(t *testing.T) {
	out := captureStdout(t, Header)
	if !strings.Contains(out, "xpm Doctor+ Report") {
		t.Errorf("header = %q, want it to contain %q", out, "xpm Doctor+ Report")
	}
	if strings.Contains(strings.ToUpper(out), "UPM") {
		t.Errorf("header mentions the old product name: %q", out)
	}
}
