package runtimes

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// githubDownloadURL is where GitHub release assets live.
var githubDownloadURL = "https://github.com"

// versionsFromTags turns release tags into versions: tags must start with
// prefix ("bun-v", "v"); drafts and canary builds are skipped.
func versionsFromTags(rels []githubRelease, prefix string) []string {
	var versions []string
	for _, r := range rels {
		v, ok := strings.CutPrefix(r.TagName, prefix)
		if !ok || r.Draft || strings.Contains(v, "canary") || env.ValidateVersionSpec(v) != nil {
			continue
		}
		versions = append(versions, v)
	}
	return versions
}

// moveToBin moves dest/<from> to dest/bin/<name>.
func moveToBin(dest, from, name string) error {
	if err := os.MkdirAll(filepath.Join(dest, "bin"), 0o755); err != nil {
		return err
	}
	return os.Rename(filepath.Join(dest, filepath.FromSlash(from)), filepath.Join(dest, "bin", name))
}
