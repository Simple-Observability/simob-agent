package hardware

type Disk struct {
	SerialNumber string `json:"serial_number"`
	Device       string `json:"device"`
	Model        string `json:"model"`
	Vendor       string `json:"vendor"`
	SizeBytes    int64  `json:"size_bytes"`
}

type Interface struct {
	MACAddress     string `json:"mac_address"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	NetworkAddress string `json:"network_address"`
	IsPhysical     bool   `json:"is_physical"`
}
