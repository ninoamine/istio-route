package cli

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultAnalysisServiceRejectsUnsupportedInputAndInvalidKubeconfig(t *testing.T) {
	service := NewDefaultAnalysisService()
	tests := []struct {
		name    string
		request Request
		wantErr string
	}{
		{
			name: "offline input is deferred",
			request: Request{
				Hostname: "shop.example.com",
				Flags:    Flags{Files: []string{"routes.yaml"}},
			},
			wantErr: "offline manifest input is not implemented yet",
		},
		{
			name: "invalid kubeconfig is surfaced",
			request: Request{
				Hostname: "shop.example.com",
				Flags:    Flags{Kubeconfig: "/path/that/does/not/exist"},
			},
			wantErr: "initialize analysis source",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Analyze(context.Background(), tt.request)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Analyze() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
