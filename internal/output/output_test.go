package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dadav/drainok/internal/analyzer"
	"github.com/dadav/drainok/internal/checks"
)

func testResults() []analyzer.NodeResult {
	return []analyzer.NodeResult{
		{Node: "control-plane", Skipped: true, SkipReason: "control-plane node"},
		{Node: "worker-1", Drainable: true},
		{Node: "worker-2", Drainable: false, Blockers: []checks.Blocker{
			{Check: "pdb", Pod: "default/web", Reason: "pod default/web is protected by PodDisruptionBudget \"web-pdb\" which allows 0 disruptions"},
			{Check: "fit", Pod: "default/big", Reason: "pod default/big does not fit on any other node"},
		}},
	}
}

func TestRenderTable(t *testing.T) {
	var sb strings.Builder
	if err := Render(&sb, "table", testResults()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"NODE", "worker-1", "yes", "worker-2", "no", "pdb:", "fit:", "skipped"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderJSONRoundTrips(t *testing.T) {
	var sb strings.Builder
	if err := Render(&sb, "json", testResults()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded []analyzer.NodeResult
	if err := json.Unmarshal([]byte(sb.String()), &decoded); err != nil {
		t.Fatalf("output is not valid json: %v", err)
	}
	if len(decoded) != 3 || decoded[2].Blockers[0].Check != "pdb" {
		t.Fatalf("unexpected decoded results: %+v", decoded)
	}
}

func TestRenderYAML(t *testing.T) {
	var sb strings.Builder
	if err := Render(&sb, "yaml", testResults()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sb.String(), "node: worker-2") {
		t.Errorf("yaml output missing node entry:\n%s", sb.String())
	}
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	var sb strings.Builder
	if err := Render(&sb, "xml", nil); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
}
