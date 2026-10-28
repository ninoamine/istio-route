package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ninoamine/istio-route/internal/kube"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func TestNewClusterSourceWithClientFactorySelectsContextAndNamespace(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		wantContext string
		wantHost    string
	}{
		{
			name:        "uses current context by default",
			wantContext: "default",
			wantHost:    "https://default.example.test",
		},
		{
			name:        "uses explicitly selected context",
			contextName: "staging",
			wantContext: "staging",
			wantHost:    "https://staging.example.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := writeKubeconfig(t)
			coreClient := kubefake.NewClientset()
			coreClient.Resources = []*metav1.APIResourceList{
				{GroupVersion: "networking.istio.io/v1"},
				{GroupVersion: "gateway.networking.k8s.io/v1"},
				{GroupVersion: "gateway.networking.k8s.io/v1beta1"},
			}
			istioClient := istiofake.NewSimpleClientset()
			gatewayClient := gatewayfake.NewClientset()
			reader := kube.NewClientForInterfaces(
				coreClient, coreClient.Discovery(), istioClient.NetworkingV1(),
				gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
			)
			var gotHost string
			factory := func(config *rest.Config) (kube.Reader, discovery.DiscoveryInterface, error) {
				gotHost = config.Host
				return reader, coreClient.Discovery(), nil
			}

			cluster, err := NewClusterSourceWithClientFactory(
				context.Background(), configPath, tt.contextName, "shop", factory,
			)
			if err != nil {
				t.Fatalf("NewClusterSourceWithClientFactory() error = %v", err)
			}
			if cluster.Context != tt.wantContext {
				t.Errorf("selected context = %q, want %q", cluster.Context, tt.wantContext)
			}
			if cluster.Namespace != "shop" {
				t.Errorf("namespace = %q, want shop", cluster.Namespace)
			}
			if gotHost != tt.wantHost {
				t.Errorf("REST config host = %q, want %q", gotHost, tt.wantHost)
			}
			snapshot, err := cluster.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if snapshot.Metadata.Context != tt.wantContext || snapshot.Metadata.Namespace != "shop" {
				t.Errorf("snapshot scope = context %q, namespace %q; want %q/shop",
					snapshot.Metadata.Context, snapshot.Metadata.Namespace, tt.wantContext)
			}
			for _, action := range istioClient.Actions() {
				if action.GetNamespace() != "shop" {
					t.Errorf("Istio list scope = %q, want shop", action.GetNamespace())
				}
			}
			for _, action := range gatewayClient.Actions() {
				if action.GetResource().Resource != "gatewayclasses" && action.GetNamespace() != "shop" {
					t.Errorf("Gateway API list scope = %q, want shop", action.GetNamespace())
				}
			}
		})
	}
}

func writeKubeconfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `apiVersion: v1
kind: Config
clusters:
  - name: default
    cluster:
      server: https://default.example.test
  - name: staging
    cluster:
      server: https://staging.example.test
contexts:
  - name: default
    context:
      cluster: default
      user: default
  - name: staging
    context:
      cluster: staging
      user: staging
current-context: default
users:
  - name: default
    user: {}
  - name: staging
    user: {}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}
