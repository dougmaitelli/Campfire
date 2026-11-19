package service

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
)

func TestDiskAccountingValidationAndLegacyResponses(t *testing.T) {
	for _, method := range []model.DiskAccounting{"", model.DiskAccountingLogical, model.DiskAccountingProject, model.DiskAccountingMixed} {
		usage := model.ResourceUsage{DiskBytes: new(int64(42)), DiskAccounting: method}
		if !validUsage(usage) {
			t.Fatal("rejected supported accounting", method)
		}

		data, err := json.Marshal(usage)
		if err != nil {
			t.Fatal(err)
		}

		if method == "" && strings.Contains(string(data), "disk_accounting") {
			t.Fatal("legacy field is not optional")
		}
	}

	if validUsage(model.ResourceUsage{DiskAccounting: "unknown"}) {
		t.Fatal("unvalidated accounting accepted")
	}
}

func TestSparkTotalsIdentifyDiskAccountingWithoutChangingHistory(t *testing.T) {
	for _, tc := range []struct {
		first, second, want model.DiskAccounting
	}{
		{"", model.DiskAccountingLogical, model.DiskAccountingLogical},
		{model.DiskAccountingProject, model.DiskAccountingProject, model.DiskAccountingProject},
		{model.DiskAccountingProject, "", model.DiskAccountingMixed},
		{model.DiskAccountingMixed, model.DiskAccountingLogical, model.DiskAccountingMixed},
	} {
		rows := []database.ServerResourceSample{
			{SparkID: "node", Usage: model.ResourceUsage{DiskBytes: new(int64(2)), DiskAccounting: tc.first}},
			{SparkID: "node", Usage: model.ResourceUsage{DiskBytes: new(int64(3)), DiskAccounting: tc.second}},
			{SparkID: "other", Usage: model.ResourceUsage{DiskBytes: new(int64(100)), DiskAccounting: model.DiskAccountingProject}},
		}

		got := aggregateResources(model.Spark{ID: "node", Connected: true}, rows, 100)
		if got.DiskBytes == nil || *got.DiskBytes != 5 || got.DiskAccounting != tc.want {
			t.Fatal(got)
		}

		rows[0].Usage.DiskError = "Counter unavailable"

		got = aggregateResources(model.Spark{ID: "node", Connected: true}, rows, 100)
		if got.DiskError == "" {
			t.Fatal("stale server reading became a fresh total")
		}

		rows[0].Usage.DiskBytes = new(int64(math.MaxInt64))

		got = aggregateResources(model.Spark{ID: "node", Connected: true}, rows, 100)
		if got.DiskBytes != nil {
			t.Fatal("disk total overflowed", got)
		}
	}

	data, err := json.Marshal(model.ResourceSample{})
	if err != nil || strings.Contains(string(data), "disk") {
		t.Fatal("disk leaked into the history contract", string(data), err)
	}
}
