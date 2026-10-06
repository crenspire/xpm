//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly)

package env

import "context"

// lockFile is a no-op where flock is unavailable (xpm env is Unix-only).
func lockFile(ctx context.Context, _ string, _ func()) (func(), error) {
	return func() {}, ctx.Err()
}
