package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildCloudflaredCommand(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		token     string
		extraArgs []string
		expected  []string
	}{
		{
			name:      "basic command without extra args",
			protocol:  "auto",
			token:     "test-token",
			extraArgs: []string{},
			expected: []string{
				"cloudflared",
				"tunnel",
				"--protocol",
				"auto",
				"--no-autoupdate",
				"--metrics",
				"0.0.0.0:44483",
				"run",
				"--token",
				"test-token",
			},
		},
		{
			name:      "command with post-quantum extra arg",
			protocol:  "quic",
			token:     "test-token",
			extraArgs: []string{"--post-quantum"},
			expected: []string{
				"cloudflared",
				"tunnel",
				"--protocol",
				"quic",
				"--no-autoupdate",
				"--post-quantum",
				"--metrics",
				"0.0.0.0:44483",
				"run",
				"--token",
				"test-token",
			},
		},
		{
			name:      "command with multiple extra args",
			protocol:  "http2",
			token:     "test-token",
			extraArgs: []string{"--post-quantum", "--edge-ip-version", "4"},
			expected: []string{
				"cloudflared",
				"--edge-ip-version",
				"4",
				"tunnel",
				"--protocol",
				"http2",
				"--no-autoupdate",
				"--post-quantum",
				"--metrics",
				"0.0.0.0:44483",
				"run",
				"--token",
				"test-token",
			},
		},
		{
			name:      "command with nil extra args",
			protocol:  "auto",
			token:     "test-token",
			extraArgs: nil,
			expected: []string{
				"cloudflared",
				"tunnel",
				"--protocol",
				"auto",
				"--no-autoupdate",
				"--metrics",
				"0.0.0.0:44483",
				"run",
				"--token",
				"test-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLOUDFLARED_TUNNEL_EDGE_IP_VERSION", "")
			result := buildCloudflaredCommand(tt.protocol, tt.token, tt.extraArgs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCloudflaredCommand_EdgeFromEnvStripsDuplicateExtra(t *testing.T) {
	t.Setenv("CLOUDFLARED_TUNNEL_EDGE_IP_VERSION", "6")
	result := buildCloudflaredCommand("auto", "tok", []string{"--edge-ip-version", "6", "--post-quantum"})
	expected := []string{
		"cloudflared", "--edge-ip-version", "6", "tunnel", "--protocol", "auto", "--no-autoupdate",
		"--post-quantum", "--metrics", "0.0.0.0:44483", "run", "--token", "tok",
	}
	assert.Equal(t, expected, result)
}

func TestSlicesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "equal slices",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "different length",
			a:        []string{"a", "b"},
			b:        []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "different content",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "x", "c"},
			expected: false,
		},
		{
			name:     "empty slices",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "nil slices",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "nil vs empty",
			a:        nil,
			b:        []string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slicesEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}