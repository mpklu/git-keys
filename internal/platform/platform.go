package platform

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Platform provides OS-specific functionality
type Platform interface {
	GetMachineID() (string, error)
	GetMachineName() (string, error)
	GetOS() string
	GetOSVersion() (string, error)
}

// macOSPlatform implements Platform for macOS
type macOSPlatform struct{}

// NewPlatform creates a new platform instance for the current OS
func NewPlatform() (Platform, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("unsupported platform: %s (only macOS is supported)", runtime.GOOS)
	}
	return &macOSPlatform{}, nil
}

// GetMachineID returns the hardware UUID on macOS
func (p *macOSPlatform) GetMachineID() (string, error) {
	// Try system_profiler first (more reliable)
	cmd := exec.Command("system_profiler", "SPHardwareDataType")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Hardware UUID:") {
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
	}

	// Fallback to ioreg
	cmd = exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get machine ID: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "IOPlatformUUID") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				return parts[3], nil
			}
		}
	}

	return "", fmt.Errorf("could not find hardware UUID")
}

// GetMachineName returns a suitable machine name
func (p *macOSPlatform) GetMachineName() (string, error) {
	cmd := exec.Command("scutil", "--get", "ComputerName")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to hostname
		cmd = exec.Command("hostname")
		output, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to get machine name: %w", err)
		}
	}
	return strings.TrimSpace(string(output)), nil
}

// GetOS returns the operating system name
func (p *macOSPlatform) GetOS() string {
	return "macOS"
}

// GetOSVersion returns the macOS version
func (p *macOSPlatform) GetOSVersion() (string, error) {
	cmd := exec.Command("sw_vers", "-productVersion")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get OS version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
