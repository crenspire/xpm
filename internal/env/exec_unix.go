//go:build !windows

package env

import "syscall"

// execProcess replaces xpm with the runtime binary; argv[0] is its absolute path.
func execProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
