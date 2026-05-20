//go:build linux

package hardware

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name       string `json:"name"`
	Serial     string `json:"serial"`
	Model      string `json:"model"`
	Vendor     string `json:"vendor"`
	Size       any    `json:"size"`
	Type       string `json:"type"`
	Tran       string `json:"tran"`
	Rota       any    `json:"rota"`
	Subsystems string `json:"subsystems"`
}

func GetPhysicalDisks() ([]Disk, error) {
	cmd := exec.Command("lsblk", "-b", "-J", "-o", "NAME,SERIAL,MODEL,VENDOR,SIZE,TYPE,TRAN,ROTA,SUBSYSTEMS")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk failed: %w", err)
	}

	var data lsblkOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	// Connect to system bus for udisks2
	conn, err := dbus.SystemBus()
	var udisksObjects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err == nil {
		defer conn.Close()
		obj := conn.Object("org.freedesktop.UDisks2", "/org/freedesktop/UDisks2")
		err = obj.Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&udisksObjects)
		if err != nil {
			udisksObjects = nil
		}
	}

	disks := []Disk{}
	for _, d := range data.BlockDevices {
		if d.Type != "disk" || isVirtual(d) {
			continue
		}

		size, _ := parseSize(d.Size)
		isSpinning := parseRota(d.Rota)

		disk := Disk{
			SerialNumber: d.Serial,
			Device:       d.Name,
			Model:        d.Model,
			Vendor:       d.Vendor,
			SizeBytes:    size,
			Interface:    strings.ToUpper(d.Tran),
			IsSpinning:   isSpinning,
			SmartStatus:  "UNKNOWN",
		}

		if udisksObjects != nil {
			enrichWithSmart(d.Name, &disk, udisksObjects)
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

func enrichWithSmart(deviceName string, disk *Disk, objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant) {
	fullDevicePath := "/dev/" + deviceName
	var drivePath dbus.ObjectPath

	// Find the Block object that matches our device name
	for _, interfaces := range objects {
		if block, ok := interfaces["org.freedesktop.UDisks2.Block"]; ok {
			if dev, ok := block["Device"]; ok {
				devStr := ""
				if b, ok := dev.Value().([]byte); ok {
					devStr = string(b)
				} else if s, ok := dev.Value().(string); ok {
					devStr = s
				}

				if strings.TrimRight(devStr, "\x00") == fullDevicePath {
					if dPath, ok := block["Drive"]; ok {
						drivePath = dPath.Value().(dbus.ObjectPath)
						break
					}
				}
			}
		}
	}

	if drivePath == "" || drivePath == "/" {
		return
	}

	// Find the Drive object and its Ata interface
	driveObj, ok := objects[drivePath]
	if !ok {
		return
	}

	ata, ok := driveObj["org.freedesktop.UDisks2.Drive.Ata"]
	if !ok {
		// Might be NVMe
		if nvme, ok := driveObj["org.freedesktop.UDisks2.NVMe.Controller"]; ok {
			disk.SmartStatus = "HEALTHY"
			if val, ok := nvme["SmartCriticalWarning"]; ok {
				if warn, ok := val.Value().([]string); ok && len(warn) > 0 {
					disk.SmartStatus = "FAILING"
				}
			}
			if val, ok := nvme["SmartTemperature"]; ok {
				if temp, ok := val.Value().(uint16); ok {
					t := float64(temp)
					disk.TemperatureK = &t
				}
			}
			if val, ok := nvme["SmartPowerOnHours"]; ok {
				if hours, ok := val.Value().(uint64); ok {
					h := int64(hours)
					disk.PowerOnHours = &h
				}
			}
		}
		return
	}

	// Extract SMART properties
	disk.SmartStatus = "HEALTHY"
	if val, ok := ata["SmartFailing"]; ok {
		if failing, ok := val.Value().(bool); ok && failing {
			disk.SmartStatus = "FAILING"
		}
	}
	if val, ok := ata["SmartTemperature"]; ok {
		if temp, ok := val.Value().(float64); ok && temp > 0 {
			disk.TemperatureK = &temp
		}
	}
	if val, ok := ata["SmartPowerOnSeconds"]; ok {
		if seconds, ok := val.Value().(uint64); ok && seconds > 0 {
			hours := int64(seconds / 3600)
			disk.PowerOnHours = &hours
		}
	}
	if val, ok := ata["SmartNumBadSectors"]; ok {
		if bad, ok := val.Value().(int64); ok && bad >= 0 {
			b := int(bad)
			disk.BadSectors = &b
		}
	}
}

func parseRota(v any) *bool {
	var val bool
	switch r := v.(type) {
	case bool:
		val = r
	case string:
		val = r == "1" || strings.ToLower(r) == "true"
	case float64:
		val = r == 1
	default:
		return nil
	}
	return &val
}

func parseSize(v any) (int64, error) {
	switch val := v.(type) {
	case float64:
		return int64(val), nil
	case int64:
		return val, nil
	case string:
		var i int64
		_, err := fmt.Sscanf(val, "%d", &i)
		return i, err
	default:
		return 0, fmt.Errorf("unknown size type: %T", v)
	}
}

func isVirtual(d lsblkDevice) bool {
	vendor := strings.ToLower(d.Vendor)
	model := strings.ToLower(d.Model)
	tran := strings.ToLower(d.Tran)
	subs := strings.ToLower(d.Subsystems)

	// Check if explicitly on a virtual bus/transport
	if tran == "virtio" || strings.Contains(subs, "virtio") || strings.Contains(subs, "xen") {
		return true
	}

	// Common virtual hardware identifiers reported in device identity
	virtualKeywords := []string{"qemu", "vbox", "vmware", "virtual disk", "virtualbox"}
	for _, kw := range virtualKeywords {
		if strings.Contains(vendor, kw) || strings.Contains(model, kw) {
			return true
		}
	}

	return false
}
