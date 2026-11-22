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

// Report contains an observation and its encoded multipart body.
type Report struct {
	Observation Observation
	SnapshotID  string
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
		if report.Observation.InventoryHash == acknowledgedHash {
			report.Observation.Status = Unchanged
		} else {
			report.Observation.Status = Snapshot
			report.SnapshotID = newSnapshotID()
		}
	}
	if err := report.encode(inventory); err != nil {
		return Report{}, err
	}
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

func (report *Report) encode(inventory []Package) error {
	metadata := struct {
		Observation
		SnapshotID   string `json:"snapshot_id,omitempty"`
		PackageCount *int   `json:"package_count,omitempty"`
	}{Observation: report.Observation, SnapshotID: report.SnapshotID}
	if report.SnapshotID != "" {
		count := len(inventory)
		metadata.PackageCount = &count
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	var body packageUpload
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("metadata", string(encoded)); err != nil {
		return err
	}
	if report.SnapshotID != "" {
		part, err := writer.CreateFormFile("data", "packages.ndjson")
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(part)
		for _, pkg := range inventory {
			if err := encoder.Encode(pkg); err != nil {
				return err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	report.ContentType = writer.FormDataContentType()
	report.Body = body.Bytes()
	return nil
}
