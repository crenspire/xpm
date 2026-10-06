package search

import (
	"fmt"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

func TestInstallManagersFor(t *testing.T) {
	for id, want := range map[pm.ID]string{
		pm.Npm:   "[npm yarn pnpm bun]",
		pm.Pip:   "[pip poetry pipenv]",
		pm.Maven: "[maven gradle]",
		pm.Cargo: "[cargo]",
	} {
		if got := fmt.Sprint(installManagersFor(id)); got != want {
			t.Errorf("installManagersFor(%s) = %s, want %s", id, got, want)
		}
	}
}
