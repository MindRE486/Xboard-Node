// Package camo serves a decoy website to TLS probes that are not valid proxy
// handshakes, and deploys the static site it serves.
//
// The site is served on a private unix socket. Kernels that support protocol
// fallbacks (Xray VLESS/Trojan over TCP+TLS) point their fallback at that
// socket, so no extra public port, reverse proxy, or panel-side setting is
// needed.
package camo

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cedar2025/xboard-node/internal/nlog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// maxTemplateSize bounds a downloaded template archive.
const maxTemplateSize = 64 << 20

// builtinIndex is served when the template download fails. It is authored by
// this project, so no third-party template is vendored into the repository.
const builtinIndex = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Welcome</title>
<style>
body{margin:0;font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f6f7f9;color:#25292e}
main{max-width:42rem;margin:18vh auto;padding:0 1.5rem}
h1{font-size:1.6rem;font-weight:600;margin:0 0 .75rem}
p{line-height:1.7;color:#5c6570;margin:.35rem 0}
</style>
</head>
<body>
<main>
<h1>Welcome</h1>
<p>This site is under construction.</p>
<p>Please check back later.</p>
</main>
</body>
</html>
`

// DeployRequest describes one decoy site deployment.
type DeployRequest struct {
	// SiteDir is the static root that must contain index.html afterwards.
	SiteDir string
	// Template selects a v2ray-agent template (1-9); 0 picks one at random.
	Template int
	// TemplateBaseURL is the directory URL that holds html<N>.zip archives.
	TemplateBaseURL string
}

// Deploy makes sure SiteDir contains a usable website. An existing index.html
// is never overwritten. Otherwise one v2ray-agent template is downloaded and
// extracted; when that fails, a built-in page is written and the failure is
// logged, so a node in a restricted network still serves a decoy.
func Deploy(req DeployRequest) error {
	if strings.TrimSpace(req.SiteDir) == "" {
		return fmt.Errorf("camouflage: site dir is empty")
	}
	if req.Template < 0 || req.Template > 9 {
		return fmt.Errorf("camouflage: template must be 0 (random) or 1-9, got %d", req.Template)
	}
	if strings.TrimSpace(req.TemplateBaseURL) == "" {
		return fmt.Errorf("camouflage: template base URL is empty")
	}

	indexPath := filepath.Join(req.SiteDir, "index.html")
	if fileExists(indexPath) {
		return nil
	}
	if err := os.MkdirAll(req.SiteDir, 0o755); err != nil {
		return fmt.Errorf("camouflage: create site dir %s: %w", req.SiteDir, err)
	}

	selected := req.Template
	if selected == 0 {
		n, err := rand.Int(rand.Reader, big.NewInt(9))
		if err != nil {
			return fmt.Errorf("camouflage: pick template: %w", err)
		}
		selected = int(n.Int64()) + 1
	}
	templateURL := fmt.Sprintf("%s/html%d.zip", strings.TrimRight(req.TemplateBaseURL, "/"), selected)

	if err := downloadAndExtract(templateURL, req.SiteDir); err != nil {
		nlog.Core().Warn("camouflage: template download failed, serving the built-in page",
			"url", templateURL, "site_dir", req.SiteDir, "error", err)
		if writeErr := os.WriteFile(indexPath, []byte(builtinIndex), 0o644); writeErr != nil {
			return fmt.Errorf("camouflage: write built-in page after %v: %w", err, writeErr)
		}
		return nil
	}
	if !fileExists(indexPath) {
		nlog.Core().Warn("camouflage: template archive has no index.html, serving the built-in page",
			"url", templateURL, "site_dir", req.SiteDir)
		if err := os.WriteFile(indexPath, []byte(builtinIndex), 0o644); err != nil {
			return fmt.Errorf("camouflage: write built-in page: %w", err)
		}
	}
	return nil
}

func downloadAndExtract(templateURL string, destDir string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(templateURL)
	if err != nil {
		return fmt.Errorf("GET %s: %w", templateURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", templateURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTemplateSize+1))
	if err != nil {
		return fmt.Errorf("read %s: %w", templateURL, err)
	}
	if len(data) > maxTemplateSize {
		return fmt.Errorf("read %s: archive exceeds %d bytes", templateURL, maxTemplateSize)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open archive %s: %w", templateURL, err)
	}
	return extract(reader, destDir)
}

// extract unpacks web assets into destDir. It rejects absolute paths, parent
// traversal, and symbolic links; macOS metadata is skipped.
func extract(reader *zip.Reader, destDir string) error {
	cleanDest := filepath.Clean(destDir)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			continue
		}
		name := file.Name
		if strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(path.Base(name), "._") || path.Base(name) == ".DS_Store" {
			continue
		}
		// Reject absolute and parent-traversal names instead of silently
		// rewriting them, so a hostile archive cannot place files outside
		// the site root or hide its intent.
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains("/"+name, "/../") || name == ".." {
			continue
		}
		cleaned := path.Clean("/" + name)
		if cleaned == "/" {
			continue
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(strings.TrimPrefix(cleaned, "/")))
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("camouflage: archive entry %q escapes %s", name, destDir)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("camouflage: create dir for %q: %w", name, err)
		}
		if err := writeZipEntry(file, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(file *zip.File, target string) error {
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("camouflage: open archive entry %q: %w", file.Name, err)
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("camouflage: create %s: %w", target, err)
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return fmt.Errorf("camouflage: write %s: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("camouflage: close %s: %w", target, err)
	}
	return nil
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

// Server is a decoy website listening on a unix socket.
type Server struct {
	listener net.Listener
	http     *http.Server
	key      string
}

// Start serves siteDir on socketPath. The socket is created with 0600 because
// only the co-located kernel dials it.
func Start(socketPath string, siteDir string) (*Server, error) {
	if strings.TrimSpace(socketPath) == "" {
		return nil, fmt.Errorf("camouflage: socket path is empty")
	}
	if !fileExists(filepath.Join(siteDir, "index.html")) {
		return nil, fmt.Errorf("camouflage: %s has no index.html", siteDir)
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, fmt.Errorf("camouflage: create socket dir: %w", err)
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("camouflage: remove stale socket %s: %w", socketPath, err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("camouflage: listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		listener.Close()
		os.Remove(socketPath)
		return nil, fmt.Errorf("camouflage: chmod socket %s: %w", socketPath, err)
	}

	server := &Server{
		listener: listener,
		key:      siteDir + "|" + socketPath,
		http: &http.Server{
			Handler:           h2c.NewHandler(siteHandler(siteDir), &http2.Server{}),
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
	go func() {
		if serveErr := server.http.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			nlog.Core().Error("camouflage: server stopped", "socket", socketPath, "error", serveErr)
		}
	}()
	nlog.Core().Info("camouflage: decoy site serving", "socket", socketPath, "site_dir", siteDir)
	return server, nil
}

// Key identifies the deployed site and socket, so a running server can be
// reused across configuration reloads.
func (s *Server) Key() string { return s.key }

// Close stops the server and removes its socket.
func (s *Server) Close() error {
	err := s.http.Close()
	if removeErr := os.Remove(s.listener.Addr().String()); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
		err = removeErr
	}
	return err
}

// siteHandler serves files from root, refuses dotfiles and directory listings,
// and lets unknown paths fall through to 404.
func siteHandler(root string) http.Handler {
	fs := http.Dir(root)
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := path.Clean("/" + r.URL.Path)
		if strings.Contains(cleaned, "/.") {
			http.NotFound(w, r)
			return
		}
		file, err := fs.Open(cleaned)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		info, statErr := file.Stat()
		file.Close()
		if statErr != nil {
			http.NotFound(w, r)
			return
		}
		if !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		index := path.Join(cleaned, "index.html")
		if !fileExists(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(index, "/")))) {
			http.NotFound(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
