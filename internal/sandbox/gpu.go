package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GPUVendor string

const (
	GPUNone   GPUVendor = ""
	GPUNvidia GPUVendor = "nvidia"
	GPUAMD    GPUVendor = "amd"
	GPUIntel  GPUVendor = "intel"
	GPUAuto   GPUVendor = "auto"
)

func ParseGPUVendor(s string) (GPUVendor, error) {
	switch s {
	case "", "none":
		return GPUNone, nil
	case "nvidia":
		return GPUNvidia, nil
	case "amd":
		return GPUAMD, nil
	case "intel":
		return GPUIntel, nil
	case "auto":
		return GPUAuto, nil
	default:
		return GPUNone, fmt.Errorf(
			"unsupported gpu vendor: %q (valid: none, nvidia, amd, intel, auto)",
			s,
		)
	}
}

func ResolveGPU(requested GPUVendor) (GPUVendor, error) {
	if requested == GPUNone ||
		requested == GPUNvidia ||
		requested == GPUAMD ||
		requested == GPUIntel {
		return requested, nil
	}

	detected := detectGPU()
	if detected == GPUNone {
		return GPUNone, fmt.Errorf("auto-detect: no GPU devices found")
	}
	return detected, nil
}

func detectGPU() GPUVendor {
	if hasNvidiaDevices() {
		return GPUNvidia
	}
	if vendor := detectDRIVendor(); vendor != GPUNone {
		return vendor
	}
	return GPUNone
}

func hasNvidiaDevices() bool {
	matches, _ := filepath.Glob("/dev/nvidia*")
	return len(matches) > 0
}

func detectDRIVendor() GPUVendor {
	entries, err := os.ReadDir("/dev/dri")
	if err != nil {
		return GPUNone
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "renderD") {
			continue
		}

		vendor := readSysfsVendor(name)
		if vendor != GPUNone {
			return vendor
		}
	}
	return GPUNone
}

func readSysfsVendor(driDevName string) GPUVendor {
	vendorPath := filepath.Join(
		"/sys/class/dri",
		driDevName,
		"device/vendor",
	)
	data, err := os.ReadFile(vendorPath)
	if err != nil {
		return GPUNone
	}

	hex := strings.TrimSpace(string(data))
	switch hex {
	case "0x1002":
		return GPUAMD
	case "0x8086":
		return GPUIntel
	case "0x10de":
		return GPUNvidia
	}
	return GPUNone
}

type GPUDriverPaths struct {
	Devices []string
	LibDirs []string
	EnvVars map[string]string
}

func GetGPUDriverPaths(vendor GPUVendor) (*GPUDriverPaths, error) {
	switch vendor {
	case GPUNvidia:
		return nvidiaDriverPaths()
	case GPUAMD:
		return amdDriverPaths()
	case GPUIntel:
		return intelDriverPaths()
	default:
		return nil, fmt.Errorf("no driver paths for gpu vendor %q", vendor)
	}
}

func nvidiaDriverPaths() (*GPUDriverPaths, error) {
	var devices []string
	for _, pattern := range []string{
		"/dev/nvidia*",
		"/dev/nvidia-uvm*",
		"/dev/nvidiactl",
		"/dev/nvidia-modeset",
	} {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if _, err := os.Stat(m); err == nil {
				devices = append(devices, m)
			}
		}
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no NVIDIA devices found in /dev")
	}

	var libDirs []string
	for _, dir := range []string{
		"/run/opengl-driver-lib",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib64",
	} {
		if _, err := os.Stat(dir); err == nil {
			libDirs = append(libDirs, dir)
		}
	}

	return &GPUDriverPaths{
		Devices: devices,
		LibDirs: libDirs,
		EnvVars: map[string]string{},
	}, nil
}

func amdDriverPaths() (*GPUDriverPaths, error) {
	return driDriverPaths("AMD")
}

func intelDriverPaths() (*GPUDriverPaths, error) {
	return driDriverPaths("Intel")
}

func driDriverPaths(vendorName string) (*GPUDriverPaths, error) {
	var devices []string

	if entries, err := os.ReadDir("/dev/dri"); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, "card") ||
				strings.HasPrefix(name, "renderD") {
				devices = append(
					devices,
					filepath.Join("/dev/dri", name),
				)
			}
		}
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no DRI devices found for %s", vendorName)
	}

	var libDirs []string
	for _, dir := range []string{
		"/run/opengl-driver-lib",
		"/usr/lib/x86_64-linux-gnu/dri",
		"/usr/lib64/dri",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib64",
	} {
		if _, err := os.Stat(dir); err == nil {
			libDirs = append(libDirs, dir)
		}
	}

	return &GPUDriverPaths{
		Devices: devices,
		LibDirs: libDirs,
		EnvVars: map[string]string{},
	}, nil
}
