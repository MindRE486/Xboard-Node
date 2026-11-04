package camo

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSite(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeployKeepsExistingSite(t *testing.T) {
	dir := t.TempDir()
	writeSite(t, dir, map[string]string{"index.html": "<h1>kept</h1>"})

	err := Deploy(DeployRequest{SiteDir: dir, Template: 1, TemplateBaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "kept") {
		t.Fatalf("existing site was overwritten: %s", body)
	}
}

func TestDeployFallsBackToBuiltinWhenDownloadFails(t *testing.T) {
	dir := t.TempDir()

	err := Deploy(DeployRequest{SiteDir: dir, Template: 3, TemplateBaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("index.html missing: %v", err)
	}
	if !strings.Contains(string(body), "Welcome") {
		t.Fatalf("built-in page not written: %s", body)
	}
}

func TestDeployExtractsArchiveAndRejectsEscapes(t *testing.T) {
	archive := zipArchive(t, map[string]string{
		"index.html":      "<h1>template</h1>",
		"assets/app.css":  "body{}",
		"../escape.txt":   "nope",
		"__MACOSX/._junk": "junk",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	dir := t.TempDir()
	if err := Deploy(DeployRequest{SiteDir: dir, Template: 2, TemplateBaseURL: server.URL}); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "app.css")); err != nil {
		t.Fatalf("asset missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive entry escaped the site dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "__MACOSX")); !os.IsNotExist(err) {
		t.Fatalf("macOS metadata was extracted")
	}
}

func TestServerServesSiteOnUnixSocket(t *testing.T) {
	site := t.TempDir()
	writeSite(t, site, map[string]string{
		"index.html":    "<h1>decoy</h1>",
		"assets/app.js": "console.log(1)",
		".env":          "SECRET=1",
	})
	socket := filepath.Join(t.TempDir(), "camo.sock")

	server, err := Start(socket, site)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Close()

	client := unixClient(socket)
	if body := get(t, client, "http://unix/"); !strings.Contains(body, "decoy") {
		t.Fatalf("index body = %q", body)
	}
	if code := status(t, client, "http://unix/assets/app.js"); code != http.StatusOK {
		t.Fatalf("asset status = %d", code)
	}
	if code := status(t, client, "http://unix/missing"); code != http.StatusNotFound {
		t.Fatalf("missing status = %d", code)
	}
	if code := status(t, client, "http://unix/.env"); code != http.StatusNotFound {
		t.Fatalf("dotfile status = %d", code)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatalf("socket left behind: %v", err)
	}
}

func zipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func unixClient(socket string) *http.Client {
	dialer := &net.Dialer{Timeout: time.Second}
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", socket)
			},
		},
	}
}

func get(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return string(body)
}

func status(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
