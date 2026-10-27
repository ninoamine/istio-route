package source

import (
	"errors"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSnapshotValidate(t *testing.T) {
	tests := []struct {
		name     string
		snapshot Snapshot
		wantErr  string
	}{
		{
			name: "empty cluster snapshot",
			snapshot: Snapshot{
				Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
			},
		},
		{
			name: "valid file resource preserves identity",
			snapshot: Snapshot{
				Metadata: model.SourceSnapshot{Mode: model.SourceModeFiles, Files: []string{"routes.yaml"}},
				Resources: []Resource{{
					Identity: model.ResourceIdentity{Version: "v1", Kind: "Service", Namespace: "shop", Name: "api"},
					GVK:      schema.GroupVersionKind{Version: "v1", Kind: "Service"},
					Object:   &corev1.Service{},
				}},
			},
		},
		{
			name: "identity and GVK must match",
			snapshot: Snapshot{
				Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
				Resources: []Resource{{
					Identity: model.ResourceIdentity{Version: "v1", Kind: "Service", Name: "api"},
					GVK:      schema.GroupVersionKind{Version: "v1", Kind: "Pod"},
					Object:   &corev1.Service{},
				}},
			},
			wantErr: "GVK does not match",
		},
		{
			name: "resource requires object",
			snapshot: Snapshot{
				Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
				Resources: []Resource{{
					Identity: model.ResourceIdentity{Version: "v1", Kind: "Service", Name: "api"},
					GVK:      schema.GroupVersionKind{Version: "v1", Kind: "Service"},
				}},
			},
			wantErr: "object is required",
		},
		{
			name: "resource requires GVK",
			snapshot: Snapshot{
				Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
				Resources: []Resource{{
					Identity: model.ResourceIdentity{Version: "v1", Kind: "Service", Name: "api"},
					Object:   &corev1.Service{},
				}},
			},
			wantErr: "group/version/kind is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.snapshot.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestError(t *testing.T) {
	cause := errors.New("connection refused")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "cluster context",
			err:  &Error{Mode: ModeCluster, Err: cause},
			want: `load cluster source: connection refused`,
		},
		{
			name: "file context",
			err:  &Error{Mode: ModeFiles, File: "routes.yaml", Err: cause},
			want: `load files source file "routes.yaml": connection refused`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
			if !errors.Is(tt.err, cause) {
				t.Fatal("Unwrap() did not preserve the source error")
			}
		})
	}
}
