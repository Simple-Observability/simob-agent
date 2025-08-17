package hostinfo

import (
	"agent/internal/version"

	"github.com/shirou/gopsutil/v4/host"
)

type HostInfo struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformFamily  string `json:"platform_family"`
	PlatformVersion string `json:"platform_version"`
	KernelVersion   string `json:"kernel_version"`
	Arch            string `json:"architecture"`
	AgentVersion    string `json:"agent_version"`
}

func Gather() (*HostInfo, error) {
	hInfo, err := host.Info()
	if err != nil {
		return nil, err
	}

	info := &HostInfo{
		Hostname:        hInfo.Hostname,
		OS:              hInfo.OS,
		Platform:        hInfo.Platform,
		PlatformFamily:  hInfo.PlatformFamily,
		PlatformVersion: hInfo.PlatformVersion,
		KernelVersion:   hInfo.KernelVersion,
		Arch:            hInfo.KernelArch,
		AgentVersion:    version.Version,
	}
	return info, nil
}
