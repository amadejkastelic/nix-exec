package sandbox

import (
	"testing"
)

func TestParseGPUVendor(t *testing.T) {
	tests := []struct {
		input string
		want  GPUVendor
		err   bool
	}{
		{"", GPUNone, false},
		{"none", GPUNone, false},
		{"nvidia", GPUNvidia, false},
		{"amd", GPUAMD, false},
		{"intel", GPUIntel, false},
		{"auto", GPUAuto, false},
		{"invalid", GPUNone, true},
		{"AMD", GPUNone, true},
		{"NVIDIA", GPUNone, true},
	}

	for _, tt := range tests {
		got, err := ParseGPUVendor(tt.input)
		if tt.err && err == nil {
			t.Errorf("ParseGPUVendor(%q) expected error, got nil", tt.input)
		}
		if !tt.err && got != tt.want {
			t.Errorf("ParseGPUVendor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveGPUPassthrough(t *testing.T) {
	tests := []struct {
		input GPUVendor
		want  GPUVendor
	}{
		{GPUNone, GPUNone},
		{GPUNvidia, GPUNvidia},
		{GPUAMD, GPUAMD},
		{GPUIntel, GPUIntel},
	}

	for _, tt := range tests {
		got, err := ResolveGPU(tt.input)
		if err != nil {
			t.Errorf("ResolveGPU(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ResolveGPU(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
