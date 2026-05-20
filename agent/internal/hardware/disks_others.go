//go:build !linux

package hardware

func GetPhysicalDisks() ([]Disk, error) {
	return []Disk{}, nil
}
