package model

import "testing"

func TestSupportsCamouflage(t *testing.T) {
	tests := []struct {
		name string
		spec *NodeSpec
		want bool
	}{
		{name: "nil", spec: nil, want: false},
		{name: "vless tcp tls", spec: &NodeSpec{Protocol: "vless", Network: "tcp", TLS: 1}, want: true},
		{name: "vless default transport tls", spec: &NodeSpec{Protocol: "vless", TLS: 1}, want: true},
		{name: "trojan tcp tls", spec: &NodeSpec{Protocol: "trojan", Network: "tcp", TLS: 1}, want: true},
		{name: "vless ws tls", spec: &NodeSpec{Protocol: "vless", Network: "ws", TLS: 1}, want: false},
		{name: "vless plaintext", spec: &NodeSpec{Protocol: "vless", Network: "tcp", TLS: 0}, want: false},
		{name: "vmess tcp tls", spec: &NodeSpec{Protocol: "vmess", Network: "tcp", TLS: 1}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.spec.SupportsCamouflage(); got != test.want {
				t.Fatalf("SupportsCamouflage = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCloneNodeSpecCopiesCamouflage(t *testing.T) {
	original := &NodeSpec{Camouflage: &CamouflageSpec{SiteDir: "/srv/site", SocketPath: "/run/camo.sock"}}
	clone := CloneNodeSpec(original)
	if clone.Camouflage == original.Camouflage {
		t.Fatal("Camouflage pointer was shared with the clone")
	}
	clone.Camouflage.SiteDir = "/changed"
	if original.Camouflage.SiteDir != "/srv/site" {
		t.Fatalf("clone mutation leaked into the original: %v", original.Camouflage)
	}
}
