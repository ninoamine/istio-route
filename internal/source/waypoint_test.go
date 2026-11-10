package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/kube"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/yaml"
)

type waypointFakes struct {
	core    *kubefake.Clientset
	gateway *gatewayfake.Clientset
	source  *ClusterSource
}

func newWaypointFakes(t *testing.T, gatewayAPI bool, objects ...runtime.Object) waypointFakes {
	t.Helper()
	core := kubefake.NewClientset()
	if gatewayAPI {
		core.Resources = append(core.Resources, &metav1.APIResourceList{GroupVersion: gatewayv1.GroupVersion.String()})
	}
	var coreObjects, gatewayObjects []runtime.Object
	for _, object := range objects {
		if _, ok := object.(*corev1.Namespace); ok {
			coreObjects = append(coreObjects, object)
		} else {
			gatewayObjects = append(gatewayObjects, object)
		}
	}
	for _, object := range coreObjects {
		if err := core.Tracker().Add(object); err != nil {
			t.Fatalf("seed core object: %v", err)
		}
	}
	gateway := gatewayfake.NewClientset()
	seedGatewayAPIReactors(gateway, gatewayObjects)
	client := kube.NewClientForInterfaces(core, core.Discovery(), nil, gateway.GatewayV1(), gateway.GatewayV1beta1())
	return waypointFakes{
		core: core, gateway: gateway,
		source: &ClusterSource{Reader: client, Discovery: client.Discovery(), Context: "staging", Namespace: "mesh"},
	}
}

// seedGatewayAPIReactors serves gets and lists from the given objects. The generated
// gateway-api fake does not register the v1 Gateway kinds used here, so plain tracker
// seeding is not reliable (see gateway_test.go, which also uses reactors).
func seedGatewayAPIReactors(client *gatewayfake.Clientset, objects []runtime.Object) {
	group := gatewayv1.GroupVersion.Group
	client.PrependReactor("get", "gateways", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.GetAction).GetName()
		for _, object := range objects {
			if gateway, ok := object.(*gatewayv1.Gateway); ok &&
				gateway.Namespace == action.GetNamespace() && gateway.Name == name {
				return true, gateway, nil
			}
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: group, Resource: "gateways"}, name)
	})
	client.PrependReactor("get", "gatewayclasses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.GetAction).GetName()
		for _, object := range objects {
			if class, ok := object.(*gatewayv1.GatewayClass); ok && class.Name == name {
				return true, class, nil
			}
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: group, Resource: "gatewayclasses"}, name)
	})
	client.PrependReactor("list", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		routes := &gatewayv1.HTTPRouteList{}
		for _, object := range objects {
			if route, ok := object.(*gatewayv1.HTTPRoute); ok {
				routes.Items = append(routes.Items, *route)
			}
		}
		return true, routes, nil
	})
}

func waypointRoute(namespace, name, parent string) *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(parent), Namespace: namespaceRef("mesh")}},
		}},
	}
}

func waypointGateway(listeners ...gatewayv1.Listener) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "mesh", Name: "waypoint"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "istio-waypoint", Listeners: listeners},
	}
}

func resourceNames(snapshot Snapshot) map[string]bool {
	names := make(map[string]bool)
	for _, resource := range snapshot.Resources {
		names[resource.Identity.Kind+"/"+resource.Identity.Namespace+"/"+resource.Identity.Name] = true
	}
	return names
}

func TestWaypointSnapshotLoadsDirectParentsAndSelectedRoute(t *testing.T) {
	fakes := newWaypointFakes(t, true,
		waypointGateway(),
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "istio-waypoint"}},
		waypointRoute("shop", "attached", "waypoint"),
		waypointRoute("shop", "detached", "elsewhere"),
		waypointRoute("other", "attached-elsewhere", "waypoint"),
	)
	snapshot, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "shop", "detached")
	if err != nil {
		t.Fatalf("WaypointSnapshot() error = %v", err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot is invalid: %v", err)
	}
	names := resourceNames(snapshot)
	for _, want := range []string{
		"Gateway/mesh/waypoint", "GatewayClass//istio-waypoint",
		"HTTPRoute/shop/attached", "HTTPRoute/shop/detached", "HTTPRoute/other/attached-elsewhere",
	} {
		if !names[want] {
			t.Errorf("snapshot is missing %s; have %v", want, names)
		}
	}
	if snapshot.Metadata.Context != "staging" || snapshot.Metadata.Namespace != "mesh" {
		t.Errorf("metadata = %#v, want context staging and namespace mesh", snapshot.Metadata)
	}
}

func TestWaypointSnapshotExcludesUnrelatedRoutes(t *testing.T) {
	fakes := newWaypointFakes(t, true,
		waypointGateway(),
		waypointRoute("shop", "elsewhere", "other-gateway"),
	)
	snapshot, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "", "")
	if err != nil {
		t.Fatalf("WaypointSnapshot() error = %v", err)
	}
	if names := resourceNames(snapshot); names["HTTPRoute/shop/elsewhere"] {
		t.Fatal("snapshot includes a route that neither targets the waypoint nor is selected")
	}
}

func TestWaypointSnapshotTolerantOfMissingGatewayClass(t *testing.T) {
	fakes := newWaypointFakes(t, true, waypointGateway())
	snapshot, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "", "")
	if err != nil {
		t.Fatalf("WaypointSnapshot() error = %v, want missing class tolerated", err)
	}
	for _, resource := range snapshot.Resources {
		if resource.Identity.Kind == "GatewayClass" {
			t.Fatal("snapshot includes a GatewayClass that does not exist")
		}
	}
}

func TestWaypointSnapshotLoadsNamespacesOnlyForSelectorListeners(t *testing.T) {
	namespaceSelector := gatewayv1.NamespacesFromSelector
	selectorListener := gatewayv1.Listener{
		Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType,
		AllowedRoutes: &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{From: &namespaceSelector}},
	}
	fakes := newWaypointFakes(t, true,
		waypointGateway(selectorListener),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}},
	)
	snapshot, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "", "")
	if err != nil {
		t.Fatalf("WaypointSnapshot() error = %v", err)
	}
	if !resourceNames(snapshot)["Namespace//shop"] {
		t.Fatal("selector listener snapshot is missing Namespace shop")
	}

	plain := newWaypointFakes(t, true, waypointGateway(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}})
	if _, err := plain.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "", ""); err != nil {
		t.Fatalf("WaypointSnapshot() error = %v", err)
	}
	for _, action := range plain.core.Actions() {
		if action.GetResource().Resource == "namespaces" {
			t.Fatal("namespaces were listed without a selector listener")
		}
	}
}

func TestWaypointSnapshotGatewayNotFound(t *testing.T) {
	fakes := newWaypointFakes(t, true)
	_, err := fakes.source.WaypointSnapshot(context.Background(), "missing", "mesh", "", "")
	if !errors.Is(err, ErrGatewayNotFound) {
		t.Fatalf("WaypointSnapshot() error = %v, want ErrGatewayNotFound", err)
	}
}

func TestWaypointSnapshotRequiresGatewayAPI(t *testing.T) {
	fakes := newWaypointFakes(t, false, waypointGateway())
	if _, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "", ""); err == nil {
		t.Fatal("WaypointSnapshot() error = nil, want Gateway API unavailable")
	}
}

func TestWaypointSnapshotSurfacesForbiddenList(t *testing.T) {
	fakes := newWaypointFakes(t, true, waypointGateway())
	fakes.gateway.PrependReactor("list", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "httproutes"}, "", errors.New("denied"))
	})
	_, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "", "")
	if !apierrors.IsForbidden(err) {
		t.Fatalf("WaypointSnapshot() error = %v, want forbidden error surfaced", err)
	}
}

func TestWaypointSnapshotHonorsCanceledContext(t *testing.T) {
	fakes := newWaypointFakes(t, true, waypointGateway())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fakes.source.WaypointSnapshot(ctx, "waypoint", "mesh", "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaypointSnapshot() error = %v, want context.Canceled", err)
	}
}

func TestWaypointSnapshotIsReadOnly(t *testing.T) {
	selector := gatewayv1.NamespacesFromSelector
	fakes := newWaypointFakes(t, true,
		waypointGateway(gatewayv1.Listener{
			Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType,
			AllowedRoutes: &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{From: &selector}},
		}),
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "istio-waypoint"}},
		waypointRoute("shop", "attached", "waypoint"),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}},
	)
	if _, err := fakes.source.WaypointSnapshot(context.Background(), "waypoint", "mesh", "shop", "attached"); err != nil {
		t.Fatalf("WaypointSnapshot() error = %v", err)
	}
	allowedVerbs := map[string]bool{"get": true, "list": true}
	// The fake discovery client records its calls as get on the pseudo-resource "resource".
	allowedResources := map[string]bool{
		"gateways": true, "gatewayclasses": true, "httproutes": true, "namespaces": true, "resource": true,
	}
	for _, client := range []*k8stesting.Fake{&fakes.core.Fake, &fakes.gateway.Fake} {
		for _, action := range client.Actions() {
			if !allowedVerbs[action.GetVerb()] {
				t.Errorf("write verb used: %s %s", action.GetVerb(), action.GetResource().Resource)
			}
			if !allowedResources[action.GetResource().Resource] {
				t.Errorf("out-of-scope resource read: %#v", action)
			}
		}
	}
}

func TestDeployRBACIsReadOnlyForWaypointResources(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "rbac.yaml"))
	if err != nil {
		t.Fatalf("read rbac.yaml: %v", err)
	}
	writeVerbs := map[string]bool{"create": true, "update": true, "patch": true, "delete": true, "deletecollection": true}
	granted := make(map[string]map[string]bool)
	for _, document := range strings.Split(string(data), "\n---\n") {
		var role rbacv1.ClusterRole
		if err := yaml.Unmarshal([]byte(document), &role); err != nil || role.Kind != "ClusterRole" {
			continue
		}
		for _, rule := range role.Rules {
			for _, verb := range rule.Verbs {
				if writeVerbs[verb] || verb == "*" {
					t.Errorf("ClusterRole %s grants write verb %q on %v", role.Name, verb, rule.Resources)
				}
			}
			for _, resource := range rule.Resources {
				if granted[resource] == nil {
					granted[resource] = make(map[string]bool)
				}
				for _, verb := range rule.Verbs {
					granted[resource][verb] = true
				}
			}
		}
	}
	for _, resource := range []string{"gateways", "gatewayclasses", "httproutes", "namespaces"} {
		if !granted[resource]["list"] && !granted[resource]["get"] {
			t.Errorf("rbac.yaml does not grant read access to %s", resource)
		}
	}
}
