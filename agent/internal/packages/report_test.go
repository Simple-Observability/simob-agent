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
	"time"

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
	if report.Snapshot == nil || report.Snapshot.ID == "" {
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

		multipartReader, err := req.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		metadataPart, err := multipartReader.NextPart()
		if err != nil || metadataPart.FormName() != "metadata" {
			t.Errorf("missing metadata part: %v", err)
			return
		}
		var metadata struct {
			packages.Observation
			SnapshotID   string `json:"snapshot_id"`
			PackageCount int    `json:"package_count"`
		}
		if err := json.NewDecoder(metadataPart).Decode(&metadata); err != nil {
			t.Error(err)
			return
		}
		if metadata.Observation != report.Observation || metadata.PackageCount != 1 || metadata.SnapshotID != report.Snapshot.ID {
			t.Errorf("unexpected metadata: %+v", metadata)
			return
		}
		inventoryPart, err := multipartReader.NextPart()
		if err != nil || inventoryPart.FormName() != "inventory" || inventoryPart.FileName() == "" {
			t.Errorf("missing inventory file: %v", err)
			return
		}
		var row packages.Package
		if err := json.NewDecoder(inventoryPart).Decode(&row); err != nil || row != rows[0] {
			t.Errorf("unexpected package row %+v: %v", row, err)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"result": "committed", "snapshot_id": metadata.SnapshotID, "inventory_hash": metadata.InventoryHash,
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
	report := packages.Report{Observation: packages.Observation{
		SchemaVersion: 1, Status: packages.Unchanged, CollectedAt: time.Now().UTC(),
		InventoryHash: packages.Hash(nil),
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Content-Type") != "application/json" {
			t.Error("expected a JSON observation")
			return
		}
		var observation packages.Observation
		if err := json.NewDecoder(req.Body).Decode(&observation); err != nil || observation != report.Observation {
			t.Errorf("unexpected observation %+v: %v", observation, err)
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
	_, err := client.PostPackageInventory(context.Background(), report)
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
