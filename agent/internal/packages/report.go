package packages

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const maxUploadBytes = 4 << 20

// Report contains an observation and its encoded JSON body.
type Report struct {
	Observation Observation
	SnapshotID  string
	Body        []byte
}

type Acknowledgement struct {
	Result        string `json:"result"`
	SnapshotID    string `json:"snapshot_id"`
	InventoryHash string `json:"inventory_hash"`
}

// NewReport chooses the wire report using the last acknowledged inventory hash.
func NewReport(inventory []Package, collectErr error, acknowledgedHash string) (Report, error) {
	report := Report{Observation: Observation{
		SchemaVersion: 1,
		CollectedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}}
	switch {
	case errors.Is(collectErr, ErrUnsupported):
		report.Observation.Status = Unsupported
	case collectErr != nil:
		return Report{}, collectErr
	default:
		report.Observation.InventoryHash = Hash(inventory)
		if report.Observation.InventoryHash == acknowledgedHash {
			report.Observation.Status = Unchanged
		} else {
			report.Observation.Status = Snapshot
			report.SnapshotID = newSnapshotID()
		}
	}
	payload := struct {
		Observation
		SnapshotID string     `json:"snapshot_id,omitempty"`
		Packages   *[]Package `json:"packages,omitempty"`
	}{Observation: report.Observation, SnapshotID: report.SnapshotID}
	if report.SnapshotID != "" {
		if inventory == nil {
			inventory = []Package{}
		}
		payload.Packages = &inventory
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Report{}, err
	}
	if len(body) > maxUploadBytes {
		return Report{}, errors.New("package upload exceeds the 4 MiB request limit")
	}
	report.Body = body
	return report, nil
}

func (report Report) ValidateAcknowledgement(ack Acknowledgement) error {
	expected := Acknowledgement{Result: "observed", InventoryHash: report.Observation.InventoryHash}
	if report.SnapshotID != "" {
		expected.Result = "committed"
		expected.SnapshotID = report.SnapshotID
	}
	if ack != expected {
		return errors.New("unexpected package inventory acknowledgement")
	}
	return nil
}

func newSnapshotID() string {
	var uuid [16]byte
	rand.Read(uuid[:])
	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}
