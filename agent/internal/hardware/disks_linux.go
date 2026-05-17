//go:build linux

package hardware

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name   string `json:"name"`
	Serial string `json:"serial"`
	Model  string `json:"model"`
	Vendor string `json:"vendor"`
	Size   any    `json:"size"`
	Type   string `json:"type"`
}

func GetPhysicalDisks() ([]Disk, error) {
	cmd := exec.Command("lsblk", "-b", "-J", "-o", "NAME,SERIAL,MODEL,VENDOR,SIZE,TYPE")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk failed: %w", err)
	}

	var data lsblkOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	var disks []Disk
	for _, d := range data.BlockDevices {
		if d.Type != "disk" {
			continue
		}

		size, _ := parseSize(d.Size)

		disks = append(disks, Disk{
			SerialNumber: d.Serial,
			Device:       d.Name,
			Model:        d.Model,
			Vendor:       d.Vendor,
			SizeBytes:    size,
		})
	}

	return disks, nil
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
