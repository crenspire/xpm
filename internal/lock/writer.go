package lock

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Marshal renders u as YAML. The output is deterministic: struct fields keep
// their declaration order and yaml.v3 sorts map keys, so equal locks always
// produce identical bytes.
func Marshal(u *UnifiedLock) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(u); err != nil {
		return nil, fmt.Errorf("marshal %s: %w", LockfileName, err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("marshal %s: %w", LockfileName, err)
	}
	return buf.Bytes(), nil
}

// WriteUnifiedLock writes u to dir/xpm-lock.yaml. When the file already holds
// exactly these bytes it is left untouched and changed is false. Otherwise the
// new content is written to a temporary file in dir and renamed over the old
// one, so a crash never leaves a half-written lock file.
func WriteUnifiedLock(dir string, u *UnifiedLock) (changed bool, err error) {
	data, err := Marshal(u)
	if err != nil {
		return false, err
	}
	target := filepath.Join(dir, LockfileName)
	if old, readErr := os.ReadFile(target); readErr == nil && bytes.Equal(old, data) {
		return false, nil
	}

	tmp, err := os.CreateTemp(dir, ".xpm-lock-*.tmp")
	if err != nil {
		return false, fmt.Errorf("write %s: %w", LockfileName, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("write %s: %w", LockfileName, err)
	}
	if err = tmp.Close(); err != nil {
		return false, fmt.Errorf("write %s: %w", LockfileName, err)
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", LockfileName, err)
	}
	if err = os.Rename(tmpName, target); err != nil {
		return false, fmt.Errorf("write %s: %w", LockfileName, err)
	}
	return true, nil
}

// ReadUnifiedLock reads dir/xpm-lock.yaml. Version 1 files (keyed by
// ecosystem) are re-keyed by each entry's file path; versions newer than
// CurrentVersion are rejected.
func ReadUnifiedLock(dir string) (*UnifiedLock, error) {
	data, err := os.ReadFile(filepath.Join(dir, LockfileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s not found - run 'xpm lock' to generate it: %w", LockfileName, err)
		}
		return nil, fmt.Errorf("read %s: %w", LockfileName, err)
	}

	var u UnifiedLock
	if err := yaml.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("parse %s: %w", LockfileName, err)
	}
	if u.Version > CurrentVersion {
		return nil, fmt.Errorf("%s has version %d; this xpm reads versions 1-%d (upgrade xpm)", LockfileName, u.Version, CurrentVersion)
	}

	locks := make(map[string]*LockInfo, len(u.Locks))
	for key, info := range u.Locks {
		if info == nil {
			info = &LockInfo{}
		}
		if info.File == "" {
			info.File = key
		}
		if u.Version <= 1 {
			key = info.File
		}
		locks[key] = info
	}
	u.Locks = locks
	return &u, nil
}

// UnifiedLockExists reports whether dir/xpm-lock.yaml exists.
func UnifiedLockExists(dir string) bool {
	return fileExists(filepath.Join(dir, LockfileName))
}

// Generate scans dir (the project root only) and builds a unified lock. A
// lockfile that cannot be read fails the whole run, so xpm-lock.yaml never
// silently omits one. Lockfiles whose packages cannot be counted are still
// recorded (Packages = 0) and explained in warnings, which the caller prints;
// the library itself writes nothing to stdout or stderr.
func Generate(dir string) (u *UnifiedLock, warnings []string, err error) {
	u = NewUnifiedLock()
	for _, d := range DetectAll(dir) {
		info, warning, err := ParseLockfile(d)
		if err != nil {
			return nil, warnings, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		u.Locks[d.RelPath] = info
	}
	return u, warnings, nil
}

// Verify compares the lockfiles on disk with dir/xpm-lock.yaml and returns one
// result per recorded entry plus one StatusAdded result per supported
// lockfile on disk that is not recorded, sorted by Key. Recorded paths that
// are absolute, contain "..", or resolve (through symlinks) outside dir yield
// StatusError and are never opened.
func Verify(dir string) ([]VerificationResult, error) {
	saved, err := ReadUnifiedLock(dir)
	if err != nil {
		return nil, err
	}

	var results []VerificationResult
	recorded := make(map[string]bool)

	for key, info := range saved.Locks {
		r := VerificationResult{Key: key, File: info.File, ExpectedHash: info.Hash}
		full, err := containedPath(dir, info.File)
		if err != nil {
			r.Status = StatusError
			r.Error = err
			results = append(results, r)
			continue
		}
		recorded[path.Clean(info.File)] = true

		if !fileExists(full) {
			r.Status = StatusMissing
			results = append(results, r)
			continue
		}
		actual, err := ComputeHash(full)
		if err != nil {
			r.Status = StatusError
			r.Error = err
			results = append(results, r)
			continue
		}
		r.ActualHash = actual
		if sameHash(info.Hash, actual) {
			r.Status = StatusUnchanged
		} else {
			r.Status = StatusChanged
		}
		results = append(results, r)
	}

	for _, d := range DetectAll(dir) {
		if recorded[d.RelPath] {
			continue
		}
		r := VerificationResult{Key: d.RelPath, File: d.RelPath, Status: StatusAdded}
		if actual, err := ComputeHash(d.Path); err == nil {
			r.ActualHash = actual
		}
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Key < results[j].Key })
	return results, nil
}

// sameHash compares a recorded hash (hex, optionally "sha256:"-prefixed, any
// case) with a computed lowercase hex hash.
func sameHash(recorded, actual string) bool {
	return strings.EqualFold(strings.TrimPrefix(recorded, "sha256:"), actual)
}
