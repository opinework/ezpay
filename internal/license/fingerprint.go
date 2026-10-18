package license

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// GenerateServerID creates a stable server fingerprint that works across
// physical machines, KVM/VMware VMs, and Docker containers.
//
// Strategy (layered, strongest first):
//  1. Collect hardware signals: MAC, disk serial, CPU ID, machine-id
//  2. If enough hardware signals → hash them (strong fingerprint)
//  3. If too few signals (Docker etc.) → use persistent file fingerprint
//     A file-based ID is generated once and stored at a configurable path.
//     As long as the file persists (volume mount), the fingerprint is stable.
//
// The fingerprint is a 32-char hex string (SHA256 first 16 bytes).
func GenerateServerID() string {
	signals := collectHardwareSignals()

	// If we have at least 2 meaningful hardware signals, use them
	if len(signals) >= 2 {
		data := strings.Join(signals, "|")
		hash := sha256.Sum256([]byte(data))
		return fmt.Sprintf("%x", hash[:16])
	}

	// Fallback: persistent file-based fingerprint (for Docker/minimal VMs)
	return getOrCreatePersistentID()
}

// collectHardwareSignals gathers all available hardware identifiers.
// Each signal is independently obtained — failures are silently skipped.
func collectHardwareSignals() []string {
	var signals []string

	// 1. MAC addresses (sorted, non-loopback, non-virtual)
	if macs := getPhysicalMACs(); len(macs) > 0 {
		sort.Strings(macs)
		signals = append(signals, macs...)
	}

	// 2. Machine ID (Linux: /etc/machine-id, unique per OS installation)
	if id := getMachineID(); id != "" {
		signals = append(signals, id)
	}

	// 3. Disk serial number
	if serial := getDiskSerial(); serial != "" {
		signals = append(signals, serial)
	}

	// 4. CPU identifier
	if cpuID := getCPUID(); cpuID != "" {
		signals = append(signals, cpuID)
	}

	// 5. OS + ARCH (weak signal but always available)
	signals = append(signals, runtime.GOOS+"/"+runtime.GOARCH)

	return signals
}

// getPhysicalMACs returns non-loopback, non-virtual MAC addresses.
// Filters out common virtual/docker bridge interfaces.
func getPhysicalMACs() []string {
	var macs []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return macs
	}

	// Prefixes for virtual interfaces to skip
	virtualPrefixes := []string{"docker", "br-", "veth", "virbr", "lo"}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		isVirtual := false
		for _, prefix := range virtualPrefixes {
			if strings.HasPrefix(name, prefix) {
				isVirtual = true
				break
			}
		}
		if isVirtual {
			continue
		}
		mac := iface.HardwareAddr.String()
		if mac != "" && mac != "00:00:00:00:00:00" {
			macs = append(macs, mac)
		}
	}
	return macs
}

// getMachineID reads the OS-level machine identifier.
// Linux: /etc/machine-id (systemd, unique per installation)
// macOS: IOPlatformUUID
// Windows: MachineGuid from registry
func getMachineID() string {
	switch runtime.GOOS {
	case "linux":
		// /etc/machine-id is stable across reboots, unique per installation
		// Docker containers get their own machine-id
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				return id
			}
		}
		// Fallback: /var/lib/dbus/machine-id
		if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
			return strings.TrimSpace(string(data))
		}
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformUUID") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						return strings.Trim(strings.TrimSpace(parts[1]), "\" ")
					}
				}
			}
		}
	case "windows":
		out, err := exec.Command("reg", "query",
			`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "MachineGuid") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						return fields[len(fields)-1]
					}
				}
			}
		}
	}
	return ""
}

// getDiskSerial returns the primary disk serial number.
func getDiskSerial() string {
	switch runtime.GOOS {
	case "linux":
		// Try common disk names
		for _, disk := range []string{"sda", "vda", "nvme0n1", "xvda"} {
			path := fmt.Sprintf("/sys/block/%s/device/serial", disk)
			if data, err := os.ReadFile(path); err == nil {
				serial := strings.TrimSpace(string(data))
				if serial != "" {
					return serial
				}
			}
		}
		// NVMe disks have serial at different path
		for _, disk := range []string{"nvme0n1"} {
			path := fmt.Sprintf("/sys/block/%s/serial", disk)
			if data, err := os.ReadFile(path); err == nil {
				serial := strings.TrimSpace(string(data))
				if serial != "" {
					return serial
				}
			}
		}
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformSerialNumber") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						return strings.Trim(strings.TrimSpace(parts[1]), "\" ")
					}
				}
			}
		}
	case "windows":
		out, err := exec.Command("wmic", "diskdrive", "get", "serialnumber").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines[1:] {
				s := strings.TrimSpace(line)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// getCPUID returns a CPU identifier string.
func getCPUID() string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Serial") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "windows":
		out, err := exec.Command("wmic", "cpu", "get", "processorid").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines[1:] {
				s := strings.TrimSpace(line)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// getOrCreatePersistentID generates a random ID and stores it in a file.
// This is the fallback for Docker containers where hardware signals are unreliable.
// The file is stored alongside the license file or in a known path.
func getOrCreatePersistentID() string {
	// Try multiple paths in order of preference
	paths := []string{
		"/etc/vendora/.server-id",         // standard install
		"/var/lib/vendora/.server-id",     // data directory
		filepath.Join(os.TempDir(), ".vendora-id"), // last resort
	}

	// Try to read existing ID
	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			id := strings.TrimSpace(string(data))
			if len(id) == 32 {
				return id
			}
		}
	}

	// Generate new random ID
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Absolute fallback — should never happen
		return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d", os.Getpid()))))[:32]
	}
	newID := hex.EncodeToString(buf)

	// Persist to the first writable path
	for _, path := range paths {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			continue
		}
		if err := os.WriteFile(path, []byte(newID), 0600); err == nil { // 0600: fingerprint file must not be world-readable (weakens BoundServerIDs)
			return newID
		}
	}

	return newID
}

// GetHostname returns the current hostname
func GetHostname() string {
	hostname, _ := os.Hostname()
	return hostname
}

// GetOSInfo returns OS information string
func GetOSInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
