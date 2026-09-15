// Package update implements release checks and self-updates for the CLI. It
// resolves the latest release tag from GitHub, downloads the archive matching
// the current platform, verifies it against SHA256SUMS, and replaces the
// running executable.
package update

import (
	"archive/tar"
	"archive/zip"
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
	"runtime"
	"strings"
	"time"
)

const (
	// Repository is the GitHub repository that publishes the release archives.
	// The template initializer rewrites this value together with the module
	// path, so initialize the template before shipping the update command.
	Repository = "github.com/pyronn/agent-cli-starter"

	// DefaultCacheTTL is how long a version check result is reused before the
	// CLI contacts the release host again.
	DefaultCacheTTL = 24 * time.Hour

	maxDownloadSize  = 256 << 20
	maxExtractedSize = 256 << 20
)

// Status describes the outcome of a version check.
type Status struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Available  bool   `json:"available"`
	CheckedAt  string `json:"checked_at"`
	ReleaseURL string `json:"release_url"`
}

// Result describes an installed release.
type Result struct {
	Previous string `json:"previous"`
	Version  string `json:"version"`
	Path     string `json:"path"`
	Updated  bool   `json:"updated"`
}

// Config configures a Service. Zero fields fall back to production defaults.
type Config struct {
	// Name is the CLI name used in release asset names, for example agentctl.
	Name string
	// Repository is the host and path of the GitHub repository.
	Repository string
	// Current is the installed version, usually the injected build version.
	Current string
	// BaseURL overrides the release host, for example https://github.com/owner/repo.
	BaseURL string
	// CachePath stores the last check result. Empty disables caching.
	CachePath string
	// CacheTTL is how long a cached check stays fresh.
	CacheTTL time.Duration
	// HTTP is the client used for release requests.
	HTTP *http.Client
	// Executable resolves the running executable path.
	Executable func() (string, error)
	// GOOS and GOARCH select the release archive to download.
	GOOS   string
	GOARCH string
	// Now returns the current time and is replaced in tests.
	Now func() time.Time
	// TempDir is the parent directory for download and extraction. Empty uses
	// the operating system temporary directory.
	TempDir string
}

// Service checks for and installs releases.
type Service struct {
	name       string
	repository string
	current    string
	baseURL    string
	cachePath  string
	cacheTTL   time.Duration
	client     *http.Client
	executable func() (string, error)
	goos       string
	goarch     string
	now        func() time.Time
	tempDir    string
}

// New builds a Service from config, filling in production defaults.
func New(config Config) *Service {
	if config.Name == "" {
		config.Name = "agentctl"
	}
	if config.Repository == "" {
		config.Repository = Repository
	}
	if config.Current == "" {
		config.Current = "dev"
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://" + config.Repository
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = DefaultCacheTTL
	}
	if config.HTTP == nil {
		config.HTTP = &http.Client{Timeout: 2 * time.Minute}
	}
	if config.Executable == nil {
		config.Executable = os.Executable
	}
	if config.GOOS == "" {
		config.GOOS = runtime.GOOS
	}
	if config.GOARCH == "" {
		config.GOARCH = runtime.GOARCH
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.CachePath == "" {
		config.CachePath = DefaultCachePath(config.Name)
	}
	return &Service{
		name:       config.Name,
		repository: config.Repository,
		current:    config.Current,
		baseURL:    strings.TrimRight(config.BaseURL, "/"),
		cachePath:  config.CachePath,
		cacheTTL:   config.CacheTTL,
		client:     config.HTTP,
		executable: config.Executable,
		goos:       config.GOOS,
		goarch:     config.GOARCH,
		now:        config.Now,
		tempDir:    config.TempDir,
	}
}

// DefaultCachePath returns the platform-native path for the update check cache.
// It returns an empty string when no user configuration directory is available,
// which disables caching.
func DefaultCachePath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, name, "update-check.json")
}

// Check reports whether a newer release exists. When useCache is true and a
// fresh cached result exists, no network request is made. Failed checks are
// cached too, so an offline machine does not retry on every invocation.
func (s *Service) Check(ctx context.Context, useCache bool) (Status, error) {
	s.removeStaleBackup()
	if useCache {
		if cached, ok := s.readCache(); ok {
			return s.status(cached.Latest, cached.CheckedAt), nil
		}
	}
	latest, err := s.latestVersion(ctx)
	checkedAt := s.now().UTC()
	if err != nil {
		s.writeCache(cacheEntry{CheckedAt: checkedAt})
		return Status{}, err
	}
	s.writeCache(cacheEntry{CheckedAt: checkedAt, Latest: latest})
	return s.status(latest, checkedAt), nil
}

// Install downloads version, verifies it against SHA256SUMS, and replaces the
// running executable. It always installs the requested version, which allows
// explicit downgrades and repairs.
func (s *Service) Install(ctx context.Context, version string, progress io.Writer) (Result, error) {
	if !validTag(version) {
		return Result{}, fmt.Errorf("invalid version %q: expected a release tag such as v1.2.3", version)
	}
	target, err := s.targetPath()
	if err != nil {
		return Result{}, err
	}

	asset := s.assetName(version)
	binary := s.binaryName()
	baseURL := s.baseURL + "/releases/download/" + version
	tempDir, err := os.MkdirTemp(s.tempDir, s.name+"-update-")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	reporter := progress
	if reporter == nil {
		reporter = io.Discard
	}
	fmt.Fprintf(reporter, "Downloading %s %s for %s/%s...\n", s.name, version, s.goos, s.goarch)

	archivePath := filepath.Join(tempDir, asset)
	if err := s.download(ctx, baseURL+"/"+asset, archivePath); err != nil {
		return Result{}, err
	}
	checksumsPath := filepath.Join(tempDir, "SHA256SUMS")
	if err := s.download(ctx, baseURL+"/SHA256SUMS", checksumsPath); err != nil {
		return Result{}, err
	}
	if err := verifyChecksum(archivePath, checksumsPath, asset); err != nil {
		return Result{}, err
	}

	extracted := filepath.Join(tempDir, binary)
	if err := extract(archivePath, binary, extracted, s.goos); err != nil {
		return Result{}, err
	}

	staged := target + ".new"
	if err := copyExecutable(extracted, staged); err != nil {
		_ = os.Remove(staged)
		return Result{}, fmt.Errorf("stage new binary: %w", err)
	}
	if err := replaceExecutable(target, staged, s.goos); err != nil {
		_ = os.Remove(staged)
		return Result{}, fmt.Errorf("replace %s: %w", target, err)
	}
	return Result{Previous: s.current, Version: version, Path: target, Updated: true}, nil
}

func (s *Service) status(latest string, checkedAt time.Time) Status {
	status := Status{
		Current:   s.current,
		Latest:    latest,
		Available: newerVersion(latest, s.current),
		CheckedAt: checkedAt.UTC().Format(time.RFC3339),
	}
	if latest != "" {
		status.ReleaseURL = s.baseURL + "/releases/tag/" + latest
	}
	return status
}

// latestVersion resolves the newest published release tag by following the
// /releases/latest redirect. This avoids the GitHub API and its rate limits.
func (s *Service) latestVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", s.userAgent())
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request latest release: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", fmt.Errorf("request latest release: %s has no published release", s.repository)
	default:
		return "", fmt.Errorf("request latest release: unexpected status %s", response.Status)
	}
	if response.Request == nil || response.Request.URL == nil {
		return "", errors.New("request latest release: missing final URL")
	}
	segments := strings.Split(strings.Trim(response.Request.URL.Path, "/"), "/")
	tag := segments[len(segments)-1]
	if !validTag(tag) {
		return "", fmt.Errorf("request latest release: unexpected release URL %s", response.Request.URL)
	}
	return tag, nil
}

func (s *Service) targetPath() (string, error) {
	path, err := s.executable()
	if err != nil {
		return "", fmt.Errorf("locate current executable: %w", err)
	}
	if path == "" {
		return "", errors.New("locate current executable: empty path")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path, nil
}

func (s *Service) assetName(version string) string {
	if s.goos == "windows" {
		return fmt.Sprintf("%s-%s-%s-%s.zip", s.name, version, s.goos, s.goarch)
	}
	return fmt.Sprintf("%s-%s-%s-%s.tar.gz", s.name, version, s.goos, s.goarch)
}

func (s *Service) binaryName() string {
	name := fmt.Sprintf("%s-%s-%s", s.name, s.goos, s.goarch)
	if s.goos == "windows" {
		name += ".exe"
	}
	return name
}

func (s *Service) userAgent() string {
	return s.name + "/" + s.current
}

func (s *Service) download(ctx context.Context, url, destination string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", s.userAgent())
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("download %s: unexpected status %s", url, response.Status)
	}
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, maxDownloadSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s: %w", url, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("download %s: %w", url, closeErr)
	}
	if written > maxDownloadSize {
		return fmt.Errorf("download %s: exceeded the %d byte limit", url, maxDownloadSize)
	}
	return nil
}

type cacheEntry struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

func (s *Service) readCache() (cacheEntry, bool) {
	if s.cachePath == "" {
		return cacheEntry{}, false
	}
	data, err := os.ReadFile(s.cachePath)
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return cacheEntry{}, false
	}
	if entry.CheckedAt.IsZero() {
		return cacheEntry{}, false
	}
	if s.now().Sub(entry.CheckedAt) > s.cacheTTL {
		return cacheEntry{}, false
	}
	return entry, true
}

func (s *Service) writeCache(entry cacheEntry) {
	if s.cachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.cachePath), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = os.WriteFile(s.cachePath, append(data, '\n'), 0o644)
}

func verifyChecksum(archivePath, checksumsPath, asset string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read SHA256SUMS: %w", err)
	}
	expected, ok := parseChecksum(data, asset)
	if !ok {
		return fmt.Errorf("SHA256SUMS does not list %s", asset)
	}
	actual, err := fileChecksum(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset, expected, actual)
	}
	return nil
}

func parseChecksum(data []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func extract(archivePath, binaryName, destination, goos string) error {
	if goos == "windows" {
		return extractZip(archivePath, binaryName, destination)
	}
	return extractTarGz(archivePath, binaryName, destination)
}

func extractZip(archivePath, binaryName, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Base(filepath.Clean(file.Name)) != binaryName {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("read %s from archive: %w", binaryName, err)
		}
		writeErr := writeExtracted(destination, source)
		closeErr := source.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return fmt.Errorf("close %s from archive: %w", binaryName, closeErr)
		}
		return nil
	}
	return fmt.Errorf("archive %s does not contain %s", archivePath, binaryName)
}

func extractTarGz(archivePath, binaryName, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read archive %s: %w", archivePath, err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive %s: %w", archivePath, err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		if filepath.Base(filepath.Clean(header.Name)) != binaryName {
			continue
		}
		return writeExtracted(destination, reader)
	}
	return fmt.Errorf("archive %s does not contain %s", archivePath, binaryName)
}

func writeExtracted(destination string, source io.Reader) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(source, maxExtractedSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("extract %s: %w", destination, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("extract %s: %w", destination, closeErr)
	}
	if written > maxExtractedSize {
		return fmt.Errorf("extract %s: exceeded the %d byte limit", destination, maxExtractedSize)
	}
	return nil
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Chmod(destination, mode|0o100)
}

// replaceExecutable moves the staged binary over the running executable. On
// Windows the running image cannot be overwritten, so the previous binary is
// renamed aside first and removed on the next successful run.
func replaceExecutable(target, staged, goos string) error {
	if goos != "windows" {
		if err := os.Chmod(staged, 0o755); err != nil {
			return err
		}
		return os.Rename(staged, target)
	}
	backup := target + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	// The previous image is still mapped by this process, so removal normally
	// fails and the stale file is cleaned up by the next invocation.
	_ = os.Remove(backup)
	return nil
}

// removeStaleBackup deletes the executable left aside by a previous Windows
// self-update. The file cannot be removed while it is still mapped.
func (s *Service) removeStaleBackup() {
	if s.goos != "windows" {
		return
	}
	path, err := s.targetPath()
	if err != nil {
		return
	}
	_ = os.Remove(path + ".old")
}
