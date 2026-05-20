package hardware

type Disk struct {
	SerialNumber string   `json:"serial_number"`
	Device       string   `json:"device"`
	Model        string   `json:"model"`
	Vendor       string   `json:"vendor"`
	SizeBytes    int64    `json:"size_bytes"`
	Interface    string   `json:"interface"`
	IsSpinning   *bool    `json:"is_spinning"`
	SmartStatus  string   `json:"smart_status"`
	TemperatureK *float64 `json:"temperature_k"`
	PowerOnHours *int64   `json:"power_on_hours"`
	BadSectors   *int     `json:"bad_sectors"`
}

type Interface struct {
	MACAddress     string `json:"mac_address"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	NetworkAddress string `json:"network_address"`
	IsPhysical     bool   `json:"is_physical"`
}
