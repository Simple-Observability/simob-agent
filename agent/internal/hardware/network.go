package hardware

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

func GetNetworkInterfaces() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	// Use a map to de-duplicate by MAC address
	byMAC := make(map[string]Interface)

	for _, iface := range ifaces {
		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		mac := iface.HardwareAddr.String()
		// Skip if no MAC address
		if mac == "" {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ipStr, networkStr string
		for _, addr := range addrs {
			ip, ipNet, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}

			// We prefer IPv4 for the primary IP/Network identification
			if ip.To4() != nil {
				ipStr = ip.String()
				networkStr = ipNet.String()
				break
			}

			// Fallback to IPv6 if we haven't found anything yet
			if ipStr == "" {
				ipStr = ip.String()
				networkStr = ipNet.String()
			}
		}

		if ipStr == "" {
			continue
		}

		newIface := Interface{
			MACAddress:     mac,
			Name:           iface.Name,
			IPAddress:      ipStr,
			NetworkAddress: networkStr,
			IsPhysical:     isPhysical(iface.Name),
		}

		// MAC de-duplication logic
		if _, ok := byMAC[mac]; !ok {
			byMAC[mac] = newIface
		}
	}

	var results []Interface
	for _, v := range byMAC {
		results = append(results, v)
	}

	return results, nil
}

func isPhysical(ifaceName string) bool {
	if runtime.GOOS != "linux" {
		return true // Default for other OSes
	}
	devicePath := filepath.Join("/sys/class/net", ifaceName, "device")
	_, err := os.Stat(devicePath)
	return err == nil
}
