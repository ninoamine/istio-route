package render

import (
	"context"
	"os"
	"testing"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/source"
)

func TestAuditRenderFormatsGolden(t *testing.T) {
	snapshot, err := (&source.FilesSource{
		Files: []string{"../../testdata/manifests/gateway-audit-healthy.yaml"},
	}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load healthy Gateway fixture: %v", err)
	}
	result, _, err := audit.Analyze(context.Background(), audit.Request{
		GatewayName: "edge", Namespace: "infra", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("analyze healthy Gateway fixture: %v", err)
	}
	tests := []struct {
		format string
		file   string
	}{
		{format: FormatText, file: "gateway-audit-healthy.txt"},
		{format: FormatJSON, file: "gateway-audit-healthy.json"},
		{format: FormatYAML, file: "gateway-audit-healthy.yaml"},
		{format: FormatMermaid, file: "gateway-audit-healthy.mmd"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, err := RenderAudit(result, tt.format, Options{})
			if err != nil {
				t.Fatalf("RenderAudit() error = %v", err)
			}
			path := "../../testdata/golden/" + tt.file
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatalf("update %s: %v", tt.file, err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", tt.file, err)
			}
			if got != string(want) {
				t.Fatalf("audit output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", tt.file, got, want)
			}
		})
	}
}
