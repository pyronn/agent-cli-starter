package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pyronn/agent-cli-starter/internal/update"
)

// releaseFixture serves a release host layout for the current platform, the
// same way GitHub serves /releases/latest, SHA256SUMS, and the archive.
type releaseFixture struct {
	server   *httptest.Server
	version  string
	binary   string
	asset    string
	content  []byte
	checksum string
}

func newReleaseFixture(t *testing.T, version string, content []byte) releaseFixture {
	t.Helper()

	goos, goarch := runtime.GOOS, runtime.GOARCH
	binary := fmt.Sprintf("%s-%s-%s", appName, goos, goarch)
	var archive []byte
	asset := ""
	if goos == "windows" {
		binary += ".exe"
		asset = fmt.Sprintf("%s-%s-%s-%s.zip", appName, version, goos, goarch)
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		file, err := writer.Create(binary)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		archive = buffer.Bytes()
	} else {
		asset = fmt.Sprintf("%s-%s-%s-%s.tar.gz", appName, version, goos, goarch)
		var buffer bytes.Buffer
		gzipWriter := gzip.NewWriter(&buffer)
		tarWriter := tar.NewWriter(gzipWriter)
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: binary, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
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
		archive = buffer.Bytes()
	}
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/releases/latest":
			http.Redirect(writer, request, "/releases/tag/"+version, http.StatusFound)
		case strings.HasPrefix(request.URL.Path, "/releases/tag/"):
			writer.WriteHeader(http.StatusOK)
		case request.URL.Path == "/releases/download/"+version+"/"+asset:
			_, _ = writer.Write(archive)
		case request.URL.Path == "/releases/download/"+version+"/SHA256SUMS":
			_, _ = io.WriteString(writer, checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	return releaseFixture{
		server: server, version: version, binary: binary, asset: asset,
		content: content, checksum: checksums,
	}
}

func (f releaseFixture) service(t *testing.T, target string) *update.Service {
	t.Helper()
	return update.New(update.Config{
		Name:       appName,
		Current:    "v1.0.0",
		BaseURL:    f.server.URL,
		HTTP:       f.server.Client(),
		CachePath:  filepath.Join(t.TempDir(), "update-check.json"),
		Executable: func() (string, error) { return target, nil },
	})
}

func TestUpdateEndToEndInstall(t *testing.T) {
	t.Parallel()

	content := []byte("agentctl v9.9.9 binary")
	fixture := newReleaseFixture(t, "v9.9.9", content)
	target := filepath.Join(t.TempDir(), fixture.binary)
	if err := os.WriteFile(target, []byte("old agentctl binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, fixture.service(t, target), "update")
	if code != 0 {
		t.Fatalf("update: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Updated agentctl to v9.9.9 (was v1.0.0)") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "Downloading agentctl v9.9.9") {
		t.Fatalf("stderr = %q, want download progress", stderr)
	}
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, content) {
		t.Fatalf("installed = %q, want %q", installed, content)
	}
}

func TestUpdateEndToEndCheck(t *testing.T) {
	t.Parallel()

	fixture := newReleaseFixture(t, "v9.9.9", []byte("payload"))
	target := filepath.Join(t.TempDir(), fixture.binary)
	if err := os.WriteFile(target, []byte("old agentctl binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, fixture.service(t, target), "--json", "update", "--check")
	if code != 0 || stderr != "" {
		t.Fatalf("update --check: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var payload struct {
		Data struct {
			Latest    string `json:"latest"`
			Available bool   `json:"available"`
			Updated   bool   `json:"updated"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	if !payload.Data.Available || payload.Data.Latest != "v9.9.9" || payload.Data.Updated {
		t.Fatalf("payload = %+v", payload.Data)
	}
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "old agentctl binary" {
		t.Fatalf("update --check modified the binary: %q", installed)
	}
}
