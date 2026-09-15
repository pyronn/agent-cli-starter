package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckReportsNewerRelease(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/releases/latest":
			http.Redirect(writer, request, "/releases/tag/v1.3.0", http.StatusFound)
		case strings.HasPrefix(request.URL.Path, "/releases/tag/"):
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	service := newTestService(t, server, Config{Current: "v1.2.0"})
	status, err := service.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.Latest != "v1.3.0" || !status.Available {
		t.Fatalf("status = %+v, want latest v1.3.0 available", status)
	}
	if status.ReleaseURL != server.URL+"/releases/tag/v1.3.0" {
		t.Fatalf("release URL = %q", status.ReleaseURL)
	}
	if status.CheckedAt == "" {
		t.Fatal("checked_at must be set")
	}
}

func TestCheckReportsUpToDate(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/releases/latest" {
			atomic.AddInt32(&requests, 1)
			http.Redirect(writer, request, "/releases/tag/v1.2.0", http.StatusFound)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := newTestService(t, server, Config{Current: "v1.2.0"})
	status, err := service.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.Available {
		t.Fatalf("status = %+v, want no available update", status)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestCheckReusesFreshCache(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/releases/latest" {
			atomic.AddInt32(&requests, 1)
			http.Redirect(writer, request, "/releases/tag/v1.3.0", http.StatusFound)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	now := time.Now()
	service := newTestService(t, server, Config{
		Current:  "v1.2.0",
		Now:      func() time.Time { return now },
		CacheTTL: time.Hour,
	})

	first, err := service.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("first Check: %v", err)
	}
	now = now.Add(30 * time.Minute)
	second, err := service.Check(context.Background(), true)
	if err != nil {
		t.Fatalf("second Check: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 (second check must use the cache)", requests)
	}
	if second.Latest != first.Latest {
		t.Fatalf("cached latest = %q, want %q", second.Latest, first.Latest)
	}

	now = now.Add(2 * time.Hour)
	if _, err := service.Check(context.Background(), true); err != nil {
		t.Fatalf("third Check: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 after the cache expired", requests)
	}
}

func TestCheckCachesFailedLookup(t *testing.T) {
	t.Parallel()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.Error(writer, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	service := newTestService(t, server, Config{Current: "v1.2.0"})
	if _, err := service.Check(context.Background(), true); err == nil {
		t.Fatal("first Check should fail")
	}
	status, err := service.Check(context.Background(), true)
	if err != nil {
		t.Fatalf("second Check should use the cached failure: %v", err)
	}
	if status.Latest != "" || status.Available {
		t.Fatalf("status = %+v, want an empty cached result", status)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestCheckReportsMissingRelease(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.NotFound(writer, request)
	}))
	defer server.Close()

	service := newTestService(t, server, Config{Current: "v1.2.0"})
	_, err := service.Check(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "no published release") {
		t.Fatalf("Check error = %v, want a missing release error", err)
	}
}

func TestCheckRejectsUnreachableHost(t *testing.T) {
	t.Parallel()

	service := New(Config{
		Name:       "agentctl",
		Current:    "v1.2.0",
		BaseURL:    "http://127.0.0.1:1",
		CachePath:  filepath.Join(t.TempDir(), "update-check.json"),
		HTTP:       &http.Client{Timeout: time.Second},
		Executable: func() (string, error) { return "", nil },
	})
	if _, err := service.Check(context.Background(), false); err == nil {
		t.Fatal("Check against an unreachable host should fail")
	}
}

func TestInstallReplacesExecutable(t *testing.T) {
	t.Parallel()

	const targetVersion = "v1.3.0"
	name := "agentctl"
	goos, goarch := runtime.GOOS, runtime.GOARCH
	binary := fmt.Sprintf("%s-%s-%s", name, goos, goarch)
	asset := fmt.Sprintf("%s-%s-%s-%s.tar.gz", name, targetVersion, goos, goarch)
	newContent := []byte("new binary payload")
	var archive []byte
	if goos == "windows" {
		binary += ".exe"
		asset = fmt.Sprintf("%s-%s-%s-%s.zip", name, targetVersion, goos, goarch)
		archive = makeZip(t, binary, newContent)
	} else {
		archive = makeTarGz(t, binary, newContent)
	}
	checksum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(checksum[:]), asset)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases/download/" + targetVersion + "/" + asset:
			_, _ = writer.Write(archive)
		case "/releases/download/" + targetVersion + "/SHA256SUMS":
			_, _ = io.WriteString(writer, checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), binary)
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	service := newTestService(t, server, Config{
		Current:    "v1.2.0",
		Executable: func() (string, error) { return target, nil },
	})
	result, err := service.Install(context.Background(), targetVersion, io.Discard)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Updated || result.Version != targetVersion || result.Path != target {
		t.Fatalf("result = %+v", result)
	}
	if result.Previous != "v1.2.0" {
		t.Fatalf("previous = %q", result.Previous)
	}
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, newContent) {
		t.Fatalf("installed content = %q, want %q", installed, newContent)
	}
	if _, err := os.Stat(target + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staged file still exists: %v", err)
	}
}

func TestInstallRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	const targetVersion = "v1.3.0"
	name := "agentctl"
	goos, goarch := runtime.GOOS, runtime.GOARCH
	binary := fmt.Sprintf("%s-%s-%s", name, goos, goarch)
	asset := fmt.Sprintf("%s-%s-%s-%s.tar.gz", name, targetVersion, goos, goarch)
	var archive []byte
	if goos == "windows" {
		binary += ".exe"
		asset = fmt.Sprintf("%s-%s-%s-%s.zip", name, targetVersion, goos, goarch)
		archive = makeZip(t, binary, []byte("tampered"))
	} else {
		archive = makeTarGz(t, binary, []byte("tampered"))
	}
	checksums := fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), asset)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases/download/" + targetVersion + "/" + asset:
			_, _ = writer.Write(archive)
		case "/releases/download/" + targetVersion + "/SHA256SUMS":
			_, _ = io.WriteString(writer, checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), binary)
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := newTestService(t, server, Config{
		Current:    "v1.2.0",
		Executable: func() (string, error) { return target, nil },
	})
	_, err := service.Install(context.Background(), targetVersion, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Install error = %v, want a checksum mismatch", err)
	}
	installed, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(installed) != "old binary" {
		t.Fatalf("target was modified: %q", installed)
	}
}

func TestInstallRejectsInvalidVersion(t *testing.T) {
	t.Parallel()

	service := New(Config{
		Name:       "agentctl",
		Current:    "v1.2.0",
		BaseURL:    "http://127.0.0.1:1",
		CachePath:  filepath.Join(t.TempDir(), "update-check.json"),
		Executable: func() (string, error) { return filepath.Join(t.TempDir(), "agentctl"), nil },
	})
	for _, value := range []string{"1.2.3", "latest", "v1.2", "dev"} {
		if _, err := service.Install(context.Background(), value, io.Discard); err == nil {
			t.Fatalf("Install(%q) should fail before any request", value)
		}
	}
}

func TestExtractZipMatchesBinaryByBaseName(t *testing.T) {
	t.Parallel()

	content := []byte("zip payload")
	archive := makeZip(t, "dist/agentctl-linux-amd64", content)
	archivePath := filepath.Join(t.TempDir(), "archive.zip")
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "agentctl")
	if err := extractZip(archivePath, "agentctl-linux-amd64", destination); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	assertFileContent(t, destination, content)

	if err := extractZip(archivePath, "missing-binary", destination); err == nil {
		t.Fatal("extractZip should fail when the binary is missing")
	}
}

func TestExtractTarGzMatchesBinaryByBaseName(t *testing.T) {
	t.Parallel()

	content := []byte("tar payload")
	archive := makeTarGz(t, "dist/agentctl-linux-amd64", content)
	archivePath := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "agentctl")
	if err := extractTarGz(archivePath, "agentctl-linux-amd64", destination); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	assertFileContent(t, destination, content)

	if err := extractTarGz(archivePath, "missing-binary", destination); err == nil {
		t.Fatal("extractTarGz should fail when the binary is missing")
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	service := New(Config{})
	if service.name != "agentctl" {
		t.Fatalf("name = %q", service.name)
	}
	if service.baseURL != "https://"+Repository {
		t.Fatalf("base URL = %q", service.baseURL)
	}
	if service.current != "dev" {
		t.Fatalf("current = %q", service.current)
	}
	if service.goos != runtime.GOOS || service.goarch != runtime.GOARCH {
		t.Fatalf("platform = %s/%s", service.goos, service.goarch)
	}
	if service.cacheTTL != DefaultCacheTTL {
		t.Fatalf("cache TTL = %s", service.cacheTTL)
	}
}

func TestNewerVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "v1.2.3", current: "v1.2.3", want: false},
		{latest: "v1.2.4", current: "v1.2.3", want: true},
		{latest: "v1.3.0", current: "v1.2.9", want: true},
		{latest: "v2.0.0", current: "v1.9.9", want: true},
		{latest: "v1.2.3", current: "v1.2.4", want: false},
		{latest: "v1.2.3-rc.1", current: "v1.2.3", want: false},
		{latest: "v1.2.3", current: "v1.2.3-rc.1", want: true},
		{latest: "v1.2.3-rc.2", current: "v1.2.3-rc.1", want: true},
		{latest: "v1.2.3-rc.1", current: "v1.2.3-beta.1", want: true},
		{latest: "v1.2.3-rc.1", current: "v1.2.3-rc.1", want: false},
		{latest: "1.2.3", current: "1.2.0", want: true},
		{latest: "v1.2.3", current: "dev", want: true},
		{latest: "dev", current: "v1.2.3", want: false},
		{latest: "v1.2", current: "v1.2.0", want: false},
	}
	for _, test := range tests {
		if actual := newerVersion(test.latest, test.current); actual != test.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", test.latest, test.current, actual, test.want)
		}
	}
}

func newTestService(t *testing.T, server *httptest.Server, config Config) *Service {
	t.Helper()
	if config.Name == "" {
		config.Name = "agentctl"
	}
	if config.BaseURL == "" {
		config.BaseURL = server.URL
	}
	if config.HTTP == nil {
		config.HTTP = server.Client()
	}
	if config.CachePath == "" {
		config.CachePath = filepath.Join(t.TempDir(), "update-check.json")
	}
	if config.Executable == nil {
		config.Executable = func() (string, error) { return filepath.Join(t.TempDir(), "agentctl"), nil }
	}
	return New(config)
}

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertFileContent(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("%s = %q, want %q", path, actual, expected)
	}
}
