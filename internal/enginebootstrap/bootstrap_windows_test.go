//go:build windows

package enginebootstrap

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func archive(t *testing.T, extra string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range []string{"chrome.exe", "chrome.dll", "chrome_elf.dll", "resources.pak", "icudtl.dat", "locales/en-US.pak", extra} {
		if name == "" {
			continue
		}
		f, err := w.Create("cloakbrowser/" + name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte("fixture-" + name))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func installerFixture(t *testing.T, b []byte, status int) (*Manager, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); w.Write(b) }))
	sum := sha256.Sum256(b)
	m := newManager(filepath.Join(t.TempDir(), Version), configuration{url: server.URL, digest: hex.EncodeToString(sum[:]), client: server.Client()})
	return m, server.Close
}

func TestMissingInstallReadyAndIntegrity(t *testing.T) {
	m, done := installerFixture(t, archive(t, ""), 200)
	defer done()
	if m.Verify() == nil {
		t.Fatal("missing engine ready")
	}
	if err := m.install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Verify(); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(m.root, manifestName)
	original, _ := os.ReadFile(manifestPath)
	if err := m.install(context.Background()); err != nil {
		t.Fatal(err)
	}
	same, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(original, same) {
		t.Fatal("already installed changed")
	}
	os.WriteFile(filepath.Join(m.root, "chrome.exe"), []byte("corrupt"), 0600)
	if m.Verify() == nil {
		t.Fatal("corrupt executable ready")
	}
	os.WriteFile(manifestPath, bytes.Replace(original, []byte(Version), []byte("other.version"), 1), 0600)
	if m.Verify() == nil {
		t.Fatal("wrong version ready")
	}
}

func TestFailedCancelledPartialHashAndRetry(t *testing.T) {
	for _, mode := range []string{"failure", "cancel", "hash", "partial"} {
		t.Run(mode, func(t *testing.T) {
			b := archive(t, "")
			status := 200
			if mode == "failure" {
				status = 503
			}
			m, done := installerFixture(t, b, status)
			defer done()
			ctx := context.Background()
			if mode == "cancel" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			if mode == "hash" {
				m.config.digest = string(bytes.Repeat([]byte("0"), 64))
			}
			if mode == "partial" {
				m.config.digest = ZipSHA256
			}
			if m.install(ctx) == nil {
				t.Fatal("failure accepted")
			}
			if m.Verify() == nil {
				t.Fatal("partial engine ready")
			}
			if _, err := os.Stat(m.root); !os.IsNotExist(err) {
				t.Fatal("failed install exposed final directory")
			}
		})
	}
	m, done := installerFixture(t, archive(t, ""), 200)
	defer done()
	bad := m.config.digest
	m.config.digest = "bad"
	if m.install(context.Background()) == nil {
		t.Fatal("accepted hash")
	}
	m.config.digest = bad
	if m.install(context.Background()) != nil {
		t.Fatal("retry failed")
	}
}

func TestUnsafeZIPAndInterruptedExtraction(t *testing.T) {
	for _, name := range []string{"../../escape", "/absolute", "C:/escape", "a\\escape", "a:stream", "CON", "COM¹", "a\x01", "trailing. ", "../escape"} {
		t.Run(name, func(t *testing.T) {
			m, done := installerFixture(t, archive(t, name), 200)
			defer done()
			if m.install(context.Background()) == nil {
				t.Fatal("unsafe zip accepted")
			}
			if m.Verify() == nil {
				t.Fatal("unsafe zip ready")
			}
		})
	}
	m, done := installerFixture(t, archive(t, ""), 200)
	defer done()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if m.extract(ctx, "missing", t.TempDir()) == nil {
		t.Fatal("interrupted extraction succeeded")
	}
}

func TestSymlinkBombDuplicateAndIncompleteStructure(t *testing.T) {
	for _, kind := range []string{"symlink", "bomb", "duplicate", "structure"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			switch kind {
			case "symlink":
				h := &zip.FileHeader{Name: "chrome.exe"}
				h.SetMode(os.ModeSymlink | 0700)
				f, _ := w.CreateHeader(h)
				f.Write([]byte("../outside"))
			case "bomb":
				h := &zip.FileHeader{Name: "chrome.exe", UncompressedSize64: 2 << 30}
				f, _ := w.CreateRaw(h)
				f.Write([]byte("x"))
			case "duplicate":
				for _, name := range []string{"chrome.exe", "CHROME.EXE"} {
					f, _ := w.Create(name)
					f.Write([]byte("x"))
				}
			case "structure":
				f, _ := w.Create("unrelated")
				f.Write([]byte("x"))
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			m, done := installerFixture(t, b.Bytes(), 200)
			defer done()
			if m.install(context.Background()) == nil {
				t.Fatal("unsafe/incomplete engine accepted")
			}
			if m.Verify() == nil {
				t.Fatal("invalid final engine exposed")
			}
		})
	}
}

func TestReadinessRejectsExtraFileAndMalformedManifest(t *testing.T) {
	m, done := installerFixture(t, archive(t, ""), 200)
	defer done()
	if err := m.install(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.root, manifestName)
	original, _ := os.ReadFile(path)
	os.WriteFile(path, append(original, []byte(" {}")...), 0600)
	if m.Verify() == nil {
		t.Fatal("trailing manifest accepted")
	}
	os.WriteFile(path, original, 0600)
	os.WriteFile(filepath.Join(m.root, "unmanifested.exe"), []byte("x"), 0600)
	if m.Verify() == nil {
		t.Fatal("unmanifested content accepted")
	}
}

func TestOfficialRedirectPolicy(t *testing.T) {
	for _, raw := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x", "https://user:pass@github.com/x", "https://release-assets.githubusercontent.com:444/x"} {
		req, _ := http.NewRequest("GET", raw, nil)
		if officialRedirect(req, nil) == nil {
			t.Fatal("unsafe redirect accepted")
		}
	}
	req, _ := http.NewRequest("GET", "https://release-assets.githubusercontent.com/x", nil)
	if officialRedirect(req, nil) != nil {
		t.Fatal("official asset host refused")
	}
}

func TestAsyncCancelAndRetry(t *testing.T) {
	payload := archive(t, "")
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		w.Write(payload)
	}))
	defer server.Close()
	sum := sha256.Sum256(payload)
	m := newManager(filepath.Join(t.TempDir(), Version), configuration{server.URL, hex.EncodeToString(sum[:]), server.Client()})
	m.Download()
	<-entered
	m.Cancel()
	select {
	case <-m.done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not complete")
	}
	if m.GetStatus().Phase != "CANCELLED" || m.Verify() == nil {
		t.Fatal("cancel produced ready engine")
	}
	close(gate)
	m.Download()
	select {
	case <-m.done:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not complete")
	}
	if !m.GetStatus().Ready {
		t.Fatal("retry not ready")
	}
}
