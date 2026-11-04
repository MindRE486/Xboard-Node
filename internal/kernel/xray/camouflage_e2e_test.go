package xray

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/model"
)

// A TLS probe that is not a valid VLESS handshake must be answered by the
// deployed decoy site, through the unix-socket fallback.
func TestVLESSFallbackServesCamouflageSite(t *testing.T) {
	siteDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>decoy-page</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(t.TempDir(), "camouflage.sock")
	addr, port := freeTCPAddress(t)
	certPEM, keyPEM := selfSignedTestCert(t)

	x := New(config.KernelConfig{Type: "xray", LogLevel: "error"})
	spec := &model.NodeSpec{
		Protocol:   "vless",
		ListenIP:   "127.0.0.1",
		ServerPort: port,
		Network:    "tcp",
		TLS:        1,
		Camouflage: &model.CamouflageSpec{SiteDir: siteDir, SocketPath: socketPath},
	}
	if err := x.Start(spec, []model.UserSpec{{ID: 1, UUID: rotationOtherUUID}}, kernel.TLSCert{CertPEM: certPEM, KeyPEM: keyPEM}); err != nil {
		t.Fatalf("start vless: %v", err)
	}
	t.Cleanup(x.Stop)

	response := tlsGet(t, addr, "/")
	if !strings.Contains(response, "200") || !strings.Contains(response, "decoy-page") {
		t.Fatalf("probe response = %q", response)
	}
	if missing := tlsGet(t, addr, "/.env"); !strings.Contains(missing, "404") {
		t.Fatalf("dotfile probe response = %q", missing)
	}
}

func freeTCPAddress(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return addr, port
}

func tlsGet(t *testing.T, addr string, requestPath string) string {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	})
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n", requestPath)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	reader := bufio.NewReader(conn)
	var builder strings.Builder
	buffer := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			builder.Write(buffer[:n])
		}
		if readErr != nil {
			break
		}
	}
	return builder.String()
}

func selfSignedTestCert(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
