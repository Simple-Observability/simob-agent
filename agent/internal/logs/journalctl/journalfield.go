package journalctl

import "encoding/json"

// JournalField handles the parsing of a systemd journal field from JSON.
// According to systemd documentation, a field can be:
// - A simple string
// - null (if too large)
// - A number array (if it contains non-UTF8/binary data)
// - An array of the above (if the field has multiple values)
// Source: https://systemd.io/JOURNAL_EXPORT_FORMATS/#journal-json-format
type JournalField struct {
	Values []string
}

// First returns the first value of the field, or an empty string if empty.
func (jf *JournalField) First() string {
	if len(jf.Values) > 0 {
		return jf.Values[0]
	}
	return ""
}

func (jf *JournalField) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	// 1. Simple string
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err == nil {
			jf.Values = []string{s}
			return nil
		}
	}

	// 2. Arrays: Can be either a byte array (single binary value) or an array of values
	if data[0] == '[' {
		var items []interface{}
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}

		if len(items) == 0 {
			return nil
		}

		// Check if it's a byte array by looking at the first element (float64)
		if _, ok := items[0].(float64); ok {
			bytesVal := make([]byte, 0, len(items))
			for _, v := range items {
				if f, ok := v.(float64); ok {
					bytesVal = append(bytesVal, byte(f))
				}
			}
			jf.Values = []string{string(bytesVal)}
			return nil
		}

		// Otherwise, it's an array of multiple values
		for _, item := range items {
			if item == nil {
				continue
			}
			if s, ok := item.(string); ok {
				jf.Values = append(jf.Values, s)
			} else if arr, ok := item.([]interface{}); ok {
				bytesVal := make([]byte, 0, len(arr))
				for _, v := range arr {
					if f, ok := v.(float64); ok {
						bytesVal = append(bytesVal, byte(f))
					}
				}
				if len(bytesVal) > 0 {
					jf.Values = append(jf.Values, string(bytesVal))
				}
			}
		}
		return nil
	}

	return nil
}
