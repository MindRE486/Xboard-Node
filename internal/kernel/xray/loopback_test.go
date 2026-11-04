package xray

// loopbackFreedomOutbound replaces the generated direct outbound with freedom
// rules that allow loopback targets. xray-core v26 blocks private destinations
// from freedom by default (SSRF protection), which also covers the loopback
// echo servers these tests proxy to. Production configs keep the upstream
// default: the generated direct outbound carries no final rules.
func loopbackFreedomOutbound() []map[string]any {
	return []map[string]any{{
		"tag":      "direct",
		"protocol": "freedom",
		"settings": map[string]any{
			"finalRules": []map[string]any{{
				"action": "allow",
				"ip":     []string{"127.0.0.0/8", "::1/128"},
			}},
		},
	}}
}
