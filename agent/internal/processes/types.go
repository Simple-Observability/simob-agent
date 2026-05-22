package processes

// ProcessInfo represents standard process monitoring metadata and dynamic statistics
type ProcessInfo struct {
	Pid     int32   `json:"pid"`
	Ppid    int32   `json:"ppid"`
	Name    string  `json:"name"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	RSS     uint64  `json:"rss"`
	Threads int32   `json:"threads"`
}
