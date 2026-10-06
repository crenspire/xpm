package runtimes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// Base URLs are variables so tests can point them at httptest servers.
var (
	nodeDistURL = "https://nodejs.org/dist"
	goDLURL     = "https://go.dev/dl"
	githubAPI   = "https://api.github.com"
)

// progress receives "Downloading ..." lines; tests silence it.
var progress io.Writer = os.Stdout

// maxSmallResponse caps metadata documents (fetchSmall/fetchJSON);
// maxReleasesResponse caps the GitHub releases list (Deno's is ~6 MB).
var (
	maxSmallResponse    int64 = 8 << 20
	maxReleasesResponse int64 = 32 << 20
)

// downloadClient bounds every runtime download: no more hanging forever on a
// stalled connection (the old code used http.Get with no timeout).
var downloadClient = &http.Client{
	Timeout: 15 * time.Minute, // whole JDKs are ~200 MB on slow links
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// statusError is a non-200 answer; isNotFound recognises 404s.
type statusError struct {
	URL  string
	Code int
}

func (e *statusError) Error() string { return fmt.Sprintf("GET %s: status %d", e.URL, e.Code) }

func isNotFound(err error) bool {
	var se *statusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

func logf(format string, args ...any) { _, _ = fmt.Fprintf(progress, format, args...) }

// get performs a ctx-bound GET and returns the response for 200 OK only.
func get(ctx context.Context, url string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", "xpm")
	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, &statusError{URL: url, Code: resp.StatusCode}
	}
	return resp, nil
}

// fetchSmall GETs a metadata document (checksum list, release JSON). It
// fails, rather than truncating, when the body exceeds 8 MiB.
func fetchSmall(ctx context.Context, url string) ([]byte, error) {
	return fetchCapped(ctx, url, nil, maxSmallResponse)
}

func fetchCapped(ctx context.Context, url string, header http.Header, limit int64) ([]byte, error) {
	resp, err := get(ctx, url, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response from %s exceeds %d MiB", url, limit>>20)
	}
	return body, nil
}

// fetchJSON decodes a small JSON document into v.
func fetchJSON(ctx context.Context, url string, v any) error {
	body, err := fetchSmall(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("parse %s: %w", url, err)
	}
	return nil
}

// githubRelease is the part of a GitHub release xpm reads.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// fetchGitHubReleases lists the newest 100 releases of repo ("owner/name"),
// authenticating with $GITHUB_TOKEN when set (60 requests/hour otherwise).
func fetchGitHubReleases(ctx context.Context, repo string) ([]githubRelease, error) {
	h := http.Header{}
	h.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		h.Set("Authorization", "Bearer "+tok)
	}
	url := githubAPI + "/repos/" + repo + "/releases?per_page=100"
	body, err := fetchCapped(ctx, url, h, maxReleasesResponse)
	if err != nil {
		return nil, err
	}
	var rels []githubRelease
	if err := json.Unmarshal(body, &rels); err != nil {
		return nil, fmt.Errorf("parse %s: %w", url, err)
	}
	return rels, nil
}

// archiveExt keeps the archive suffix of a URL so extractArchive can
// dispatch on the downloaded file's name.
func archiveExt(url string) string {
	base := path.Base(url)
	for _, ext := range []string{".tar.gz", ".tgz", ".zip"} {
		if strings.HasSuffix(base, ext) {
			return ext
		}
	}
	return ""
}

// downloadVerified streams url to a temp file while hashing it, and returns
// the temp path only if its SHA-256 equals wantHex. On any failure the temp
// file is removed. The caller must os.Remove the returned path when done.
func downloadVerified(ctx context.Context, url, wantHex string) (string, error) {
	return downloadVerifiedTo(ctx, url, wantHex, "")
}

// downloadVerifiedTo is downloadVerified with the temp file in dir
// ("" = os.TempDir()), for files that must be executed (rustup-init).
func downloadVerifiedTo(ctx context.Context, url, wantHex, dir string) (string, error) {
	if len(wantHex) != sha256.Size*2 {
		return "", fmt.Errorf("refusing to download %s: no valid SHA-256 to verify against", url)
	}
	logf("Downloading %s\n", url)
	resp, err := get(ctx, url, nil)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(dir, "xpm-dl-*"+archiveExt(url))
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp.Name())
		if copyErr != nil {
			return "", fmt.Errorf("download %s: %w", url, copyErr)
		}
		return "", closeErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantHex) {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", url, got, wantHex)
	}
	return tmp.Name(), nil
}

// checksumFromSums finds filename in a SHASUMS256.txt-style document
// ("<hex>  <filename>" per line; a leading '*' on the name marks binary mode).
func checksumFromSums(sums, filename string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no published checksum for %s", filename)
}

// goChecksum finds filename's sha256 in go.dev/dl/?mode=json&include=all output.
func goChecksum(releasesJSON []byte, filename string) (string, error) {
	var releases []struct {
		Files []struct {
			Filename string `json:"filename"`
			SHA256   string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(releasesJSON, &releases); err != nil {
		return "", fmt.Errorf("parse Go release list: %w", err)
	}
	for _, r := range releases {
		for _, f := range r.Files {
			if f.Filename == filename {
				return f.SHA256, nil
			}
		}
	}
	return "", fmt.Errorf("no published checksum for %s", filename)
}
