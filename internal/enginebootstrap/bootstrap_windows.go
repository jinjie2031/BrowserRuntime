//go:build windows

// Package enginebootstrap installs one pinned, officially downloaded engine.
// It never adopts the shared CloakBrowser cache and never executes ZIP content.
package enginebootstrap

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"browserruntime/internal/portable"
)

const Version = "146.0.7680.177.5"
const DownloadURL = "https://github.com/CloakHQ/CloakBrowser/releases/download/chromium-v146.0.7680.177.5/cloakbrowser-windows-x64.zip"
const ZipSHA256 = "b213795cb32c3169f766c74ce1d0275fc89d3df256de39c04da7fb4c23b7fdbe"
const manifestName = ".browserruntime-engine.json"
const maxDownload int64 = 700 << 20
const maxExpanded uint64 = 2 << 30

var ErrNotReady = errors.New("CloakBrowser engine is not ready")
var ErrDownload = errors.New("Official engine download failed")
var required = []string{"chrome.exe", "chrome.dll", "chrome_elf.dll", "resources.pak", "icudtl.dat", "locales/en-US.pak"}

type Status struct {
	Enabled  bool   `json:"enabled"`
	Ready    bool   `json:"ready"`
	Phase    string `json:"phase"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
}
type configuration struct {
	url, digest string
	client      *http.Client
}
type manifest struct {
	Schema        int               `json:"schema"`
	Version       string            `json:"version"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	Files         map[string]string `json:"files"`
}
type Manager struct {
	root   string
	config configuration
	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
	done   chan struct{}
}

func officialRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Port() != "" {
		return ErrDownload
	}
	switch req.URL.Hostname() {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return nil
	}
	return ErrDownload
}

func New(root string) *Manager {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // Engine acquisition does not inherit arbitrary environment proxies.
	transport.ResponseHeaderTimeout = 30 * time.Second
	return newManager(root, configuration{DownloadURL, ZipSHA256, &http.Client{Transport: transport, Timeout: 45 * time.Minute, CheckRedirect: officialRedirect}})
}
func newManager(root string, c configuration) *Manager {
	return &Manager{root: root, config: c, status: Status{Enabled: true, Phase: "CHECKING"}}
}
func (m *Manager) Executable() string { return filepath.Join(m.root, "chrome.exe") }
func (m *Manager) GetStatus() Status  { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
func (m *Manager) Check() {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	ready := m.Verify() == nil
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel == nil {
		m.status.Ready = ready
		if ready {
			m.status.Phase = "READY"
		} else {
			m.status.Phase = "MISSING"
		}
	}
}
func (m *Manager) Download() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil || m.status.Ready {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	m.status = Status{Enabled: true, Phase: "DOWNLOADING"}
	go func() {
		err := m.install(ctx)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cancel = nil
		if err == nil {
			m.status.Ready = true
			m.status.Phase = "READY"
		} else if ctx.Err() != nil {
			m.status.Phase = "CANCELLED"
		} else {
			m.status.Phase = "FAILED"
		}
		cancel()
		close(m.done)
	}()
}
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
}
func (m *Manager) Shutdown() {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}
func (m *Manager) phase(phase string) { m.mu.Lock(); m.status.Phase = phase; m.mu.Unlock() }

func safeName(s string) bool {
	if s == "" || len(s) > 1024 || strings.ContainsAny(s, `\:`) || strings.HasPrefix(s, "/") || strings.ContainsFunc(s, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(s, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		switch base {
		case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
			return false
		}
		if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return false
		}
		if (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.ContainsAny(strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT"), "¹²³") {
			return false
		}
		if strings.ContainsAny(part, `<>"|?*`) {
			return false
		}
	}
	return true
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", ErrNotReady
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", ErrNotReady
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (m *Manager) Verify() error {
	if portable.CheckTree(m.root) != nil {
		return ErrNotReady
	}
	f, err := os.Open(filepath.Join(m.root, manifestName))
	if err != nil {
		return ErrNotReady
	}
	defer f.Close()
	var v manifest
	info, statErr := f.Stat()
	if statErr != nil || info.Size() > 4<<20 {
		return ErrNotReady
	}
	d := json.NewDecoder(io.LimitReader(f, 4<<20))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil || v.Schema != 1 || v.Version != Version || v.ArchiveSHA256 != m.config.digest || len(v.Files) < len(required) || len(v.Files) > 10000 {
		return ErrNotReady
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return ErrNotReady
	}
	for _, name := range required {
		if v.Files[name] == "" {
			return ErrNotReady
		}
	}
	for name, expected := range v.Files {
		if !safeName(name) || len(expected) != 64 {
			return ErrNotReady
		}
		actual, err := hashFile(filepath.Join(m.root, filepath.FromSlash(name)))
		if err != nil || actual != expected {
			return ErrNotReady
		}
	}
	// Reject added executables/resources or hidden partial content as well.
	count := 0
	err = filepath.WalkDir(m.root, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return ErrNotReady
		}
		if !e.IsDir() {
			rel, _ := filepath.Rel(m.root, p)
			if rel != manifestName {
				count++
				if v.Files[filepath.ToSlash(rel)] == "" {
					return ErrNotReady
				}
			}
		}
		return nil
	})
	if err != nil || count != len(v.Files) {
		return ErrNotReady
	}
	return nil
}

type progressWriter struct {
	m *Manager
	w io.Writer
}

func (p progressWriter) Write(b []byte) (int, error) {
	n, e := p.w.Write(b)
	p.m.mu.Lock()
	p.m.status.Received += int64(n)
	p.m.mu.Unlock()
	return n, e
}

func (m *Manager) install(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrDownload
	}
	if m.Verify() == nil {
		return nil
	}
	parent := filepath.Dir(m.root)
	if portable.CheckTree(parent) != nil || os.MkdirAll(parent, 0700) != nil {
		return ErrDownload
	}
	temp, err := os.MkdirTemp(parent, ".download-")
	if err != nil {
		return ErrDownload
	}
	defer os.RemoveAll(temp) // only this freshly created, private staging directory
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.config.url, nil)
	if err != nil {
		return ErrDownload
	}
	response, err := m.config.client.Do(req)
	if err != nil {
		return ErrDownload
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxDownload {
		return ErrDownload
	}
	m.mu.Lock()
	m.status.Total = response.ContentLength
	m.mu.Unlock()
	zipPath := filepath.Join(temp, "engine.zip")
	file, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrDownload
	}
	h := sha256.New()
	n, err := io.Copy(progressWriter{m, io.MultiWriter(file, h)}, io.LimitReader(response.Body, maxDownload+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || n > maxDownload || (response.ContentLength >= 0 && n != response.ContentLength) || hex.EncodeToString(h.Sum(nil)) != m.config.digest || ctx.Err() != nil {
		return ErrDownload
	}
	m.phase("EXTRACTING")
	staging := filepath.Join(temp, "engine")
	if os.Mkdir(staging, 0700) != nil {
		return ErrDownload
	}
	if err = m.extract(ctx, zipPath, staging); err != nil {
		return ErrDownload
	}
	candidate := newManager(staging, m.config)
	if candidate.Verify() != nil || ctx.Err() != nil {
		return ErrDownload
	}
	if portable.CheckTree(parent) != nil {
		return ErrDownload
	}
	// A previously invalid instance engine is quarantined, never merged or overwritten.
	if _, err := os.Lstat(m.root); err == nil {
		quarantine, err := os.MkdirTemp(parent, ".invalid-")
		if err != nil {
			return ErrDownload
		}
		if os.Rename(m.root, filepath.Join(quarantine, "engine")) != nil {
			return ErrDownload
		}
	}
	if ctx.Err() != nil {
		return ErrDownload
	}
	if os.Rename(staging, m.root) != nil {
		return ErrDownload
	}
	return nil
}

func (m *Manager) extract(ctx context.Context, zipPath, staging string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return ErrDownload
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > 10000 {
		return ErrDownload
	}
	// Official archive contains one top-level directory. Also accept a flat archive
	// for injected tests; never search recursively for an arbitrary executable.
	prefix := ""
	for _, f := range reader.File {
		if strings.HasSuffix(f.Name, "/chrome.exe") {
			prefix = strings.TrimSuffix(f.Name, "chrome.exe")
			break
		}
	}
	if prefix != "" && strings.Count(strings.TrimSuffix(prefix, "/"), "/") != 0 {
		return ErrDownload
	}
	seen := map[string]bool{}
	files := map[string]string{}
	var total uint64
	for _, f := range reader.File {
		if ctx.Err() != nil {
			return ErrDownload
		}
		if !safeName(f.Name) || f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return ErrDownload
		}
		name := strings.TrimPrefix(f.Name, prefix)
		if name == "" && f.FileInfo().IsDir() {
			continue
		}
		if prefix != "" && !strings.HasPrefix(f.Name, prefix) {
			return ErrDownload
		}
		if !safeName(name) || strings.EqualFold(name, manifestName) {
			return ErrDownload
		}
		lower := strings.ToLower(strings.TrimSuffix(name, "/"))
		if seen[lower] {
			return ErrDownload
		}
		seen[lower] = true
		if f.UncompressedSize64 > 1<<30 || total > maxExpanded-f.UncompressedSize64 {
			return ErrDownload
		}
		total += f.UncompressedSize64
		dest := filepath.Join(staging, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if os.MkdirAll(dest, 0700) != nil {
				return ErrDownload
			}
			continue
		}
		if os.MkdirAll(filepath.Dir(dest), 0700) != nil {
			return ErrDownload
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return ErrDownload
		}
		in, err := f.Open()
		if err != nil {
			out.Close()
			return ErrDownload
		}
		h := sha256.New()
		n, copyErr := copyContext(ctx, io.MultiWriter(out, h), io.LimitReader(in, int64(f.UncompressedSize64)+1))
		in.Close()
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil || uint64(n) != f.UncompressedSize64 {
			return ErrDownload
		}
		files[name] = hex.EncodeToString(h.Sum(nil))
	}
	for _, name := range required {
		if files[name] == "" {
			return ErrDownload
		}
	}
	data, err := json.Marshal(manifest{1, Version, m.config.digest, files})
	if err != nil {
		return ErrDownload
	}
	if ctx.Err() != nil {
		return ErrDownload
	}
	return os.WriteFile(filepath.Join(staging, manifestName), data, 0600)
}

func copyContext(ctx context.Context, w io.Writer, r io.Reader) (int64, error) {
	b := make([]byte, 128<<10)
	var total int64
	for {
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
		n, e := r.Read(b)
		if n > 0 {
			written, err := w.Write(b[:n])
			total += int64(written)
			if err != nil {
				return total, err
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if e == io.EOF {
			return total, nil
		}
		if e != nil {
			return total, e
		}
	}
}
