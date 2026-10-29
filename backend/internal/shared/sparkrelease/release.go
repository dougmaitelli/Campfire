// Package sparkrelease downloads and verifies Spark packages from the connected panel.
package sparkrelease

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const releaseDownloadTimeout = 5 * time.Minute
const maximumArchiveBytes = 256 << 20
const maximumExpandedBytes = 512 << 20
const maximumBinaryBytes = 128 << 20

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

var developmentPattern = regexp.MustCompile(`^dev-[a-f0-9]{7,64}$`)

// ValidVersion accepts release identifiers and commit-pinned development builds.
func ValidVersion(version string) bool {
	return version == "dev" || versionPattern.MatchString(version) || developmentPattern.MatchString(version)
}

// UpdateAvailable allows newer releases, development builds to move to a release,
// or a different commit on the development channel.
func UpdateAvailable(current, target string) bool {
	if current == "dev" || developmentPattern.MatchString(current) {
		return current != target && (versionPattern.MatchString(target) || developmentPattern.MatchString(target))
	}

	return Newer(current, target)
}

// Newer compares numeric version components, excluding development labels.
func Newer(current, latest string) bool {
	a, b := versionPattern.FindStringSubmatch(current), versionPattern.FindStringSubmatch(latest)
	if len(a) == 0 || len(b) == 0 {
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

	if a[4] == "" || a[4] == b[4] {
		return false
	}

	if b[4] == "" {
		return true
	}

	left, right := strings.Split(a[4][1:], "."), strings.Split(b[4][1:], ".")
	for index := range min(len(left), len(right)) {
		if left[index] == right[index] {
			continue
		}

		aNumeric, bNumeric := numeric(left[index]), numeric(right[index])
		if aNumeric != bNumeric {
			return aNumeric
		}

		if aNumeric && len(left[index]) != len(right[index]) {
			return len(right[index]) > len(left[index])
		}

		return right[index] > left[index]
	}

	return len(right) > len(left)
}

func numeric(value string) bool {
	return value != "" && strings.Trim(value, "0123456789") == ""
}

// Client uses bounded HTTPS requests; tests can supply an isolated transport.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// NewClient uses the panel origin and its existing trusted TLS transport.
func NewClient(baseURL string, transport http.RoundTripper) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Transport: transport, Timeout: releaseDownloadTimeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("spark package redirects are not allowed")
	}}}
}

func (c *Client) get(ctx context.Context, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Campfire-Spark-Updater")
	req.Header.Set("Accept", "application/octet-stream")

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

func archiveName(arch string) string {
	return "spark_linux_" + arch + ".tar.gz"
}

// Stage downloads and verifies an archive, extracting only its regular Spark binary.
// The returned private temporary path is on the destination filesystem.
func (c *Client) Stage(ctx context.Context, version, arch, directory string) (string, error) {
	if !ValidVersion(version) || (arch != "amd64" && arch != "arm64") {
		return "", errors.New("unsupported Spark release or architecture")
	}

	address, err := url.Parse(c.BaseURL)
	if err != nil || address.Scheme != "https" || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return "", errors.New("spark packages require a trusted HTTPS panel origin")
	}

	name := archiveName(arch)
	base := c.BaseURL + "/" + version + "/" + name

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
