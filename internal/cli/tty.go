package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/crenspire/xpm/internal/pm"
)

// isInteractiveTerminal reports whether prompts can work: both stdin and
// stdout are terminals. A seam for tests.
var isInteractiveTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// effectiveInteractive is whether xpm may prompt: the config allows it and
// a terminal is attached. Pipes, CI and `xpm ... | cat` never hang on a prompt.
func effectiveInteractive(configured, tty bool) bool {
	return configured && tty
}

// refuseGuessWhenUnavailable returns an error when a registry did not answer:
// the missing hit might have been the right one, so a non-interactive run
// must not pick for the user. It returns nil when every registry answered.
func refuseGuessWhenUnavailable(unavailable []pm.ID) error {
	if len(unavailable) == 0 {
		return nil
	}
	names := make([]string, len(unavailable))
	for i, id := range unavailable {
		names[i] = managerName(id)
	}
	return fmt.Errorf("not choosing automatically because %s did not answer and the list of matches may be incomplete; retry, or turn that registry off with `xpm config set search.%s false`",
		strings.Join(names, ", "), unavailable[0])
}
