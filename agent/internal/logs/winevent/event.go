//go:build windows

package winevent

import (
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agent/internal/logs"
)

//go:embed event_ids.json
var eventMapData []byte // Source: https://www.ultimatewindowssecurity.com/securitylog/encyclopedia/default.aspx
var eventLookup map[int]string

func init() {
	// Initialize the map from the embedded JSON file
	if err := json.Unmarshal(eventMapData, &eventLookup); err != nil {
		eventLookup = make(map[int]string)
	}
}

type WinEventLog struct {
	XMLName xml.Name `xml:"Event"`

	System struct {
		EventID     int `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`

	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
}

func (e *WinEventLog) Labels() map[string]string {
	return map[string]string{}
}

func (e *WinEventLog) Metadata() map[string]string {
	metadata := make(map[string]string)
	metadata["EventId"] = strconv.Itoa(e.System.EventID)
	metadata["Channel"] = "Security"

	// Add EventData fields
	for _, data := range e.EventData.Data {
		val := data.Value

		// Skip fields with no value or name
		if data.Name == "" || val == "" {
			continue
		}

		// Filter out placeholders
		if val == "-" || val == "0x0" || val == "{00000000-0000-0000-0000-000000000000}" {
			continue
		}

		// Skip internal Hex IDs. These change every session.
		lowContextKeys := map[string]bool{
			"SubjectLogonId":      true,
			"TargetLogonId":       true,
			"TargetLinkedLogonId": true,
			"LogonGuid":           true,
			"TransmittedServices": true,
		}
		if lowContextKeys[data.Name] {
			continue
		}

		// Humanize Windows "%%" tokens
		// From multiple sources:
		// https://learn.microsoft.com/en-us/archive/msdn-technet-forums/340632d1-60f0-4cc5-ad6f-f8c841107d0d
		// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/auditing/event-4688
		if strings.HasPrefix(val, "%%") {
			switch val {
			case "%%1832":
				val = "Identification"
			case "%%1833":
				val = "Impersonation"
			case "%%1840":
				val = "Delegation"
			case "%%1841":
				val = "Denied by Process Trust Label ACE"
			case "%%1842":
				val = "Yes"
			case "%%1843":
				val = "No"
			case "%%1844":
				val = "System"
			case "%%1845":
				val = "Not Available"
			case "%%1846":
				val = "Default"
			case "%%1847":
				val = "DisallowMmConfig"
			case "%%1848":
				val = "Off"
			case "%%1849":
				val = "Auto"
			case "%%1936":
				val = "Type 1 (Full Token)"
			case "%%1937":
				val = "Type 2 (Elevated Token)"
			case "%%1938":
				val = "Type 3 (Limited Token)"
			}
		}

		metadata[data.Name] = val
	}
	return metadata
}

func (e *WinEventLog) Message() string {
	message, ok := eventLookup[e.System.EventID]
	if !ok {
		message = "Windows Event"
	}
	for i, data := range e.EventData.Data {
		placeholder := fmt.Sprintf("%%%d", i+1)
		message = strings.ReplaceAll(message, placeholder, data.Value)
	}
	return message
}

func (e *WinEventLog) ToLogEntry() logs.LogEntry {
	// Parse timestamp
	var timestamp int64
	t, err := time.Parse(time.RFC3339Nano, e.System.TimeCreated.SystemTime)
	if err == nil {
		timestamp = t.UnixMilli()
	} else {
		// Fallback to current time if parse fails
		timestamp = time.Now().UnixMilli()
	}

	return logs.LogEntry{
		Timestamp: timestamp,
		Source:    "winevent",
		Text:      e.Message(),
		Labels:    e.Labels(),
		Metadata:  e.Metadata(),
	}
}
