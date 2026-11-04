package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/model"
)

func TestAttachCamouflageWiresSupportedTarget(t *testing.T) {
	configDir := t.TempDir()
	siteDir := filepath.Join(t.TempDir(), "www")
	s := &Service{cfg: &config.Config{
		Kernel: config.KernelConfig{ConfigDir: configDir},
		Camouflage: &config.CamouflageConfig{
			Enabled:         true,
			SiteDir:         siteDir,
			Template:        1,
			TemplateBaseURL: "http://127.0.0.1:1",
		},
	}}
	spec := &model.NodeSpec{Protocol: "vless", Network: "tcp", TLS: 1}

	if err := s.attachCamouflage(spec, model.KernelXray); err != nil {
		t.Fatalf("attachCamouflage: %v", err)
	}
	if spec.Camouflage == nil {
		t.Fatal("supported target was not wired")
	}
	if spec.Camouflage.SiteDir != siteDir {
		t.Fatalf("site dir = %q", spec.Camouflage.SiteDir)
	}
	if spec.Camouflage.SocketPath != filepath.Join(configDir, "camouflage.sock") {
		t.Fatalf("socket path = %q", spec.Camouflage.SocketPath)
	}
	if _, err := os.Stat(filepath.Join(siteDir, "index.html")); err != nil {
		t.Fatalf("decoy site was not deployed: %v", err)
	}
}

func TestAttachCamouflageSkipsUnsupportedTargets(t *testing.T) {
	s := &Service{cfg: &config.Config{
		Kernel:     config.KernelConfig{ConfigDir: t.TempDir()},
		Camouflage: &config.CamouflageConfig{Enabled: true, SiteDir: t.TempDir(), TemplateBaseURL: "http://127.0.0.1:1"},
	}}
	cases := []struct {
		name   string
		spec   *model.NodeSpec
		kernel string
	}{
		{name: "singbox kernel", spec: &model.NodeSpec{Protocol: "vless", Network: "tcp", TLS: 1}, kernel: model.KernelSingbox},
		{name: "ws transport", spec: &model.NodeSpec{Protocol: "vless", Network: "ws", TLS: 1}, kernel: model.KernelXray},
		{name: "plaintext", spec: &model.NodeSpec{Protocol: "vless", Network: "tcp", TLS: 0}, kernel: model.KernelXray},
		{name: "vmess", spec: &model.NodeSpec{Protocol: "vmess", Network: "tcp", TLS: 1}, kernel: model.KernelXray},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := s.attachCamouflage(test.spec, test.kernel); err != nil {
				t.Fatalf("attachCamouflage: %v", err)
			}
			if test.spec.Camouflage != nil {
				t.Fatalf("unsupported target was wired: %#v", test.spec.Camouflage)
			}
		})
	}
}
