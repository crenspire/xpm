package runtimes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	nodeDistURL = "https://nodejs.org/dist"
	goDLURL     = "https://go.dev/dl"
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

// fetchSmall GETs a metadata document (checksum list, release JSON), capped at 8 MiB.
func fetchSmall(url string) ([]byte, error) {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// downloadVerified streams url to a temp file while hashing it, and returns
// the temp path only if its SHA-256 equals wantHex. On any failure the temp
// file is removed. The caller must os.Remove the returned path when done.
func downloadVerified(url, wantHex string) (string, error) {
	if len(wantHex) != sha256.Size*2 {
		return "", fmt.Errorf("refusing to download %s: no valid SHA-256 to verify against", url)
	}
	resp, err := downloadClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "xpm-dl-*")
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
