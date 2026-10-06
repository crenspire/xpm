//go:build windows

package env

import "errors"

// execProcess is unavailable: xpm env (and its shims) are Unix-only.
func execProcess(string, []string, []string) error {
	return errors.New("xpm shims are not supported on Windows")
}
