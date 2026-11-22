package packages

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"time"
)

// Report contains an observation and an encoded upload when a snapshot is needed.
type Report struct {
	Observation Observation
	Snapshot    *SnapshotUpload
}

type SnapshotUpload struct {
	ID          string
	ContentType string
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
		report.Observation.Status = Snapshot
		if report.Observation.InventoryHash == acknowledgedHash {
			report.Observation.Status = Unchanged
		} else {
			snapshot, err := encodePackageSnapshot(report.Observation, inventory)
			if err != nil {
				return Report{}, err
			}
			report.Snapshot = snapshot
		}
	}
	return report, nil
}

func (report Report) ValidateAcknowledgement(ack Acknowledgement) error {
	expected := Acknowledgement{Result: "observed", InventoryHash: report.Observation.InventoryHash}
	if report.Snapshot != nil {
		expected.Result = "committed"
		expected.SnapshotID = report.Snapshot.ID
	}
	if ack != expected {
		return errors.New("unexpected package inventory acknowledgement")
	}
	return nil
}

var errPackageUploadTooLarge = errors.New("package upload exceeds the 32 MiB request limit")

type packageUpload struct {
	bytes.Buffer
}

func (body *packageUpload) Write(data []byte) (int, error) {
	if len(data) > MaxUploadBytes-body.Len() {
		return 0, errPackageUploadTooLarge
	}
	return body.Buffer.Write(data)
}

func newSnapshotID() string {
	var uuid [16]byte
	rand.Read(uuid[:])
	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

func encodePackageSnapshot(observation Observation, inventory []Package) (*SnapshotUpload, error) {
	snapshotID := newSnapshotID()
	metadata, err := json.Marshal(struct {
		Observation
		SnapshotID   string `json:"snapshot_id"`
		PackageCount int    `json:"package_count"`
	}{
		Observation: observation, SnapshotID: snapshotID, PackageCount: len(inventory),
	})
	if err != nil {
		return nil, err
	}

	var body packageUpload
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("metadata", string(metadata)); err != nil {
		return nil, err
	}
	part, err := writer.CreateFormFile("inventory", "packages.ndjson")
	if err != nil {
		return nil, err
	}
	encoder := json.NewEncoder(part)
	for _, p := range inventory {
		if err := encoder.Encode(p); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return &SnapshotUpload{ID: snapshotID, ContentType: writer.FormDataContentType(), Body: body.Bytes()}, nil
}
