package packages_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"agent/internal/api"
	"agent/internal/config"
	"agent/internal/logger"
	"agent/internal/packages"
)

func TestPostPackageInventorySendsSnapshotOnce(t *testing.T) {
	logger.Init(false)
	rows := []packages.Package{{
		Manager: "dpkg", Name: "nginx-core", Version: "1.24.0-2ubuntu7.19", Arch: "amd64",
		SourceName: "nginx", SourceVersion: "1.24.0-2ubuntu7.19",
	}}
	report, err := packages.NewReport(rows, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.SnapshotID == "" {
		t.Fatal("expected a snapshot with an ID")
	}
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts.Add(1)
		if req.Method != http.MethodPost || req.URL.Path != "/servers/packages/" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			return
		}
		if req.Header.Get("Authorization") != "Api-Key test-key" {
			t.Error("missing or incorrect authorization header")
			return
		}

		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("unexpected content type: %s", got)
			return
		}
		var payload struct {
			packages.Observation
			SnapshotID string             `json:"snapshot_id"`
			Packages   []packages.Package `json:"packages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if payload.Observation != report.Observation || payload.SnapshotID != report.SnapshotID {
			t.Errorf("unexpected snapshot: %+v", payload)
			return
		}
		if len(payload.Packages) != 1 || payload.Packages[0] != rows[0] {
			t.Errorf("unexpected package list: %+v", payload.Packages)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"result": "committed", "snapshot_id": payload.SnapshotID, "inventory_hash": payload.InventoryHash,
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	client := api.NewClient(config.Config{APIUrl: server.URL, APIKey: "test-key"}, false)
	if acknowledged, err := client.PostPackageInventory(context.Background(), report); err != nil || !acknowledged {
		t.Fatalf("snapshot was not acknowledged: %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected one upload attempt, got %d", got)
	}
}

func TestPostPackageInventoryRequiresAcknowledgementAndHonorsDryRun(t *testing.T) {
	logger.Init(false)
	report, err := packages.NewReport(nil, nil, packages.Hash(nil))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var payload struct {
			packages.Observation
			Packages json.RawMessage `json:"packages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Observation != report.Observation {
			t.Errorf("unexpected observation %+v: %v", payload, err)
			return
		}
		if payload.Packages != nil {
			t.Errorf("unchanged reports must omit packages, got %s", payload.Packages)
			return
		}
		_, _ = io.WriteString(w, `{"result":"observed","inventory_hash":"other"}`)
	}))
	defer server.Close()

	client := api.NewClient(config.Config{APIUrl: server.URL}, false)
	if acknowledged, err := client.PostPackageInventory(context.Background(), report); err == nil || acknowledged {
		t.Fatal("unexpected acknowledgement for a different hash")
	}

	var attempts atomic.Int32
	rejectingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts.Add(1)
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
	}))
	defer rejectingServer.Close()

	cfg := config.Config{APIUrl: rejectingServer.URL}
	client = api.NewClient(cfg, false)
	_, err = client.PostPackageInventory(context.Background(), report)
	if err == nil || !strings.Contains(err.Error(), "status 413") {
		t.Fatalf("expected status 413 error, got %v", err)
	}

	dry := api.NewClient(cfg, true)
	if acknowledged, err := dry.PostPackageInventory(context.Background(), report); err != nil || acknowledged {
		t.Fatalf("dry run returned %t, %v", acknowledged, err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected one request including dry run, got %d", got)
	}
}

func TestNewReportDistinguishesEmptySnapshotsFromUnsupportedReports(t *testing.T) {
	for _, test := range []struct {
		status     string
		collectErr error
	}{
		{status: packages.Snapshot},
		{status: packages.Unsupported, collectErr: packages.ErrUnsupported},
	} {
		t.Run(test.status, func(t *testing.T) {
			report, err := packages.NewReport(nil, test.collectErr, "")
			if err != nil {
				t.Fatal(err)
			}
			var fields struct {
				packages.Observation
				Packages json.RawMessage `json:"packages"`
			}
			if err := json.Unmarshal(report.Body, &fields); err != nil || fields.Status != test.status {
				t.Fatalf("unexpected report: %+v, %v", fields, err)
			}
			if test.status == packages.Snapshot {
				if string(fields.Packages) != "[]" {
					t.Fatalf("expected an empty package list, got %s", fields.Packages)
				}
			} else if fields.Packages != nil {
				t.Fatal("unsupported reports must omit packages")
			}
		})
	}
}

func TestNewReportRejectsOversizedJSON(t *testing.T) {
	inventory := make([]packages.Package, 20000)
	for i := range inventory {
		inventory[i] = packages.Package{
			Manager: "dpkg", Name: "nginx-core", Version: "1.24", Arch: "amd64",
			SourceName: "nginx", SourceVersion: "1." + strings.Repeat("a", 200),
		}
	}
	if _, err := packages.NewReport(inventory, nil, ""); err == nil || !strings.Contains(err.Error(), "4 MiB") {
		t.Fatalf("expected an upload limit error, got %v", err)
	}
}
