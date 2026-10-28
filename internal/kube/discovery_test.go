package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDiscoverGroupVersion(t *testing.T) {
	tests := []struct {
		name          string
		resources     []*metav1.APIResourceList
		reactor       k8stesting.ReactionFunc
		wantAvailable bool
		wantReason    bool
		wantErr       string
	}{
		{
			name: "available",
			resources: []*metav1.APIResourceList{{
				GroupVersion: "gateway.networking.k8s.io/v1",
			}},
			wantAvailable: true,
		},
		{
			name:       "missing API is skipped",
			wantReason: true,
		},
		{
			name: "authorization error is returned",
			reactor: func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("forbidden")
			},
			wantErr: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fake.FakeDiscovery{Fake: &k8stesting.Fake{}}
			client.Resources = tt.resources
			if tt.reactor != nil {
				client.PrependReactor("get", "resource", tt.reactor)
			}
			available, reason, err := DiscoverGroupVersion(
				context.Background(), client, "gateway.networking.k8s.io/v1",
			)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("DiscoverGroupVersion() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DiscoverGroupVersion() error = %v", err)
			}
			if available != tt.wantAvailable {
				t.Errorf("available = %t, want %t", available, tt.wantAvailable)
			}
			if (reason != "") != tt.wantReason {
				t.Errorf("reason = %q, want reason present=%t", reason, tt.wantReason)
			}
		})
	}
}

func TestDiscoverGroupVersionHonorsContextAndRequiresClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := DiscoverGroupVersion(ctx, nil, "example.test/v1"); !errors.Is(err, context.Canceled) {
		t.Errorf("DiscoverGroupVersion(canceled) error = %v, want context.Canceled", err)
	}
	if _, _, err := DiscoverGroupVersion(context.Background(), nil, "example.test/v1"); err == nil {
		t.Error("DiscoverGroupVersion(nil client) succeeded")
	}
}

func TestDiscoverGroupVersionDoesNotSuppressUnexpectedAPIError(t *testing.T) {
	client := &fake.FakeDiscovery{Fake: &k8stesting.Fake{}}
	client.PrependReactor("get", "resource", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "resource"}, "v1", errors.New("denied"),
		)
	})
	if available, reason, err := DiscoverGroupVersion(
		context.Background(), client, "example.test/v1",
	); available || reason != "" || err == nil {
		t.Fatalf("DiscoverGroupVersion() = %t, %q, %v; want surfaced error", available, reason, err)
	}
}
