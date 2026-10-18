// Package sparkrelease reads and verifies official Spark release packages.
package sparkrelease

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const repositoryName = "dougmaitelli/Campfire"
const repository = "https://github.com/" + repositoryName
const latestURL = "https://api.github.com/repos/" + repositoryName + "/releases/latest"
const releaseDownloadTimeout = 5 * time.Minute
const maximumArchiveBytes = 256 << 20
const maximumExpandedBytes = 512 << 20
const maximumBinaryBytes = 128 << 20

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// StableVersion accepts only full stable release tags.
func StableVersion(version string) bool {
	parts := versionPattern.FindStringSubmatch(version)
	return len(parts) != 0 && strings.HasPrefix(version, "v") && parts[4] == "" && parts[5] == ""
}

// Newer compares numeric version components, excluding development labels.
func Newer(current, latest string) bool {
	a, b := versionPattern.FindStringSubmatch(current), versionPattern.FindStringSubmatch(latest)
	if len(a) == 0 || len(b) == 0 || !StableVersion(latest) {
		return false
	}

	for index := 1; index <= 3; index++ {
		if len(a[index]) != len(b[index]) {
			return len(b[index]) > len(a[index])
		}

		if a[index] != b[index] {
			return b[index] > a[index]
		}
	}

	return a[4] != ""
}

// Client uses bounded HTTPS requests; tests can supply an isolated transport.
type Client struct{ HTTP *http.Client }

// NewClient creates an official-release transport.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: releaseDownloadTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 5 {
			return errors.New("invalid release redirect")
		}

		return nil
	}}}
}

func (c *Client) get(ctx context.Context, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Campfire-Spark-Updater")
	req.Header.Set("Accept", "application/vnd.github+json")

	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("release request returned HTTP %d", response.StatusCode)
	}

	return response, nil
}

// Latest returns the published stable release only when both packages exist.
func (c *Client) Latest(ctx context.Context) (string, error) {
	response, err := c.get(ctx, latestURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()

	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return "", err
	}

	if release.Draft || release.Prerelease || !StableVersion(release.Tag) {
		return "", errors.New("no stable Spark release is available")
	}

	assets := map[string]bool{}
	for _, asset := range release.Assets {
		assets[asset.Name] = true
	}

	for _, arch := range []string{"amd64", "arm64"} {
		name := archiveName(release.Tag, arch)
		if !assets[name] || !assets[name+".sha256"] {
			return "", errors.New("spark release packages are not yet published")
		}
	}

	return release.Tag, nil
}

func archiveName(version, arch string) string {
	return "campfire_" + version + "_linux_" + arch + ".tar.gz"
}

// Stage downloads and verifies an archive, extracting only its regular Spark binary.
// The returned private temporary path is on the destination filesystem.
func (c *Client) Stage(ctx context.Context, version, arch, directory string) (string, error) {
	if !StableVersion(version) || (arch != "amd64" && arch != "arm64") {
		return "", errors.New("unsupported Spark release or architecture")
	}

	name := archiveName(version, arch)
	base := repository + "/releases/download/" + version + "/" + name

	checksum, err := c.get(ctx, base+".sha256")
	if err != nil {
		return "", err
	}

	checksumData, err := io.ReadAll(io.LimitReader(checksum.Body, 4096))
	_ = checksum.Body.Close()

	if err != nil {
		return "", err
	}

	fields := strings.Fields(string(checksumData))
	if len(fields) != 2 || fields[1] != name || len(fields[0]) != 64 {
		return "", errors.New("invalid release checksum")
	}

	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("invalid release checksum")
	}

	archive, err := os.CreateTemp(directory, ".spark-archive-*")
	if err != nil {
		return "", err
	}

	defer func() { _ = archive.Close(); _ = os.Remove(archive.Name()) }()

	response, err := c.get(ctx, base)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(response.Body, maximumArchiveBytes+1))
	_ = response.Body.Close()

	if err != nil {
		return "", err
	}

	if size > maximumArchiveBytes || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(fields[0]) {
		return "", errors.New("spark archive checksum verification failed")
	}

	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	zipped, err := gzip.NewReader(archive)
	if err != nil {
		return "", err
	}

	defer func() { _ = zipped.Close() }()

	reader := tar.NewReader(io.LimitReader(zipped, maximumExpandedBytes+1))
	for {
		header, err := reader.Next()
		if err != nil {
			return "", errors.New("spark binary is missing from the release archive")
		}

		if header.Name != "spark" && header.Name != "./spark" {
			continue
		}

		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maximumBinaryBytes {
			return "", errors.New("invalid Spark binary archive entry")
		}

		binary, err := os.CreateTemp(directory, ".spark-update-*")
		if err != nil {
			return "", err
		}

		filename := binary.Name()
		_, copyErr := io.Copy(binary, reader)
		syncErr := binary.Sync()

		closeErr := binary.Close()
		if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
			_ = os.Remove(filename)
			return "", err
		}

		if err := os.Chmod(filename, 0755); err != nil {
			_ = os.Remove(filename)
			return "", err
		}

		return filepath.Clean(filename), nil
	}
}
