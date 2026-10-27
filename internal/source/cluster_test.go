package source

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/kube"
	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func TestClusterSourceSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		discovered    []string
		reactor       k8stesting.ReactionFunc
		wantErr       string
		wantFamilies  int
		wantResources int
	}{
		{
			name: "loads all supported resource families",
			discovered: []string{
				"networking.istio.io/v1",
				"gateway.networking.k8s.io/v1",
				"gateway.networking.k8s.io/v1beta1",
			},
			wantFamilies:  3,
			wantResources: 8,
		},
		{
			name:         "continues when CRD families are absent",
			wantFamilies: 3,
		},
		{
			name: "surfaces discovery authorization failures",
			reactor: func(action k8stesting.Action) (bool, runtime.Object, error) {
				if action.GetVerb() == "get" && action.GetResource().Resource == "resource" {
					return true, nil, errors.New("forbidden")
				}
				return false, nil, nil
			},
			wantErr: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreClient := kubefake.NewClientset()
			for _, groupVersion := range tt.discovered {
				coreClient.Resources = append(coreClient.Resources, &metav1.APIResourceList{
					GroupVersion: groupVersion,
				})
			}
			if tt.reactor != nil {
				coreClient.PrependReactor("*", "*", tt.reactor)
			}
			istioClient := istiofake.NewSimpleClientset(
				&istiov1.VirtualService{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "shop"}},
				&istiov1.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
				&istiov1.ServiceEntry{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "external"}},
			)
			istioClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, &istiov1.GatewayList{Items: []*istiov1.Gateway{{
					ObjectMeta: metav1.ObjectMeta{Namespace: "istio-system", Name: "edge"},
				}}}, nil
			})
			gatewayClient := gatewayfake.NewClientset(
				&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "istio"}},
				&gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "route"}},
				&gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "grant"}},
			)
			gatewayClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, &gatewayv1.GatewayList{Items: []gatewayv1.Gateway{{
					ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "edge"},
				}}}, nil
			})
			client := kube.NewClientForInterfaces(
				coreClient, coreClient.Discovery(), istioClient.NetworkingV1(),
				gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
			)
			src := ClusterSource{
				Reader: client, Discovery: client.Discovery(),
				Context: "staging", Namespace: "",
			}

			snapshot, err := src.Snapshot(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Snapshot() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if len(snapshot.Metadata.Families) != tt.wantFamilies {
				t.Fatalf("snapshot families = %d, want %d", len(snapshot.Metadata.Families), tt.wantFamilies)
			}
			if len(snapshot.Resources) != tt.wantResources {
				t.Fatalf("snapshot resources = %d, want %d: %#v", len(snapshot.Resources), tt.wantResources, snapshot.Resources)
			}
			if err := snapshot.Validate(); err != nil {
				t.Fatalf("Snapshot.Validate() error = %v", err)
			}
			if snapshot.Metadata.Context != "staging" {
				t.Errorf("snapshot context = %q, want staging", snapshot.Metadata.Context)
			}
			if tt.wantResources > 0 {
				got := snapshot.Resources[0]
				if got.Identity.Group == "" || got.Identity.Version == "" || got.Identity.Kind == "" || got.Identity.Name == "" {
					t.Errorf("resource identity was not preserved: %#v", got.Identity)
				}
				if got.GVK != (schema.GroupVersionKind{
					Group: got.Identity.Group, Version: got.Identity.Version, Kind: got.Identity.Kind,
				}) {
					t.Errorf("resource GVK = %#v, identity = %#v", got.GVK, got.Identity)
				}
			}
		})
	}
}

func TestClusterSourceSnapshotRequiresClients(t *testing.T) {
	_, err := (&ClusterSource{}).Snapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reader and discovery") {
		t.Fatalf("Snapshot() error = %v, want missing-client error", err)
	}
}
