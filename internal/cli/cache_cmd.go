package cli

import (
	"fmt"
	"os"
)

// cacheRemovedMsg explains why `xpm cache` (and the cc/cg aliases) no longer
// do anything. The dependency cache was never used by installs and has been
// deleted; each package manager keeps its own cache.
const cacheRemovedMsg = `xpm cache has been removed: package managers keep their own caches.
Clear them with the tool itself, for example:
  npm cache clean --force
  yarn cache clean
  pnpm store prune
  pip cache purge
  composer clear-cache
  cargo clean (per project)
  go clean -modcache`

// cmdCache reports that the cache command was removed and fails.
func cmdCache(_ []string) int {
	fmt.Fprintln(os.Stderr, cacheRemovedMsg)
	return 1
}
