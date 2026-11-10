package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/simulate"
	"github.com/ninoamine/istio-route/internal/source"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

type fakeSimulateService struct {
	result  simulate.Result
	status  simulate.Status
	err     error
	request SimulateRequest
}

func (s *fakeSimulateService) Simulate(_ context.Context, request SimulateRequest) (simulate.Result, simulate.Status, error) {
	s.request = request
	return s.result, s.status, s.err
}

func runSimulate(t *testing.T, service SimulateService, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	command := newSimulateCommand(service, &out, &errOut)
	command.SetArgs(args)
	err = command.Execute()
	return out.String(), errOut.String(), err
}

func TestSimulateCommandPassesRequest(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantRoute *simulate.RouteRef
		wantFiles []string
		wantName  string
	}{
		{
			name:     "listing without route",
			args:     []string{"waypoint", "-f", "manifests.yaml"},
			wantName: "waypoint", wantFiles: []string{"manifests.yaml"},
		},
		{
			name:      "impact with route",
			args:      []string{"--route", "shop/reviews", "-n", "app", "waypoint"},
			wantName:  "waypoint",
			wantRoute: &simulate.RouteRef{Namespace: "shop", Name: "reviews"},
		},
		{
			name:      "stdin manifests",
			args:      []string{"-f", "-", "waypoint"},
			wantName:  "waypoint",
			wantFiles: []string{"-"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeSimulateService{status: simulate.StatusFound}
			// Text rendering is not implemented yet; only the request is under test here.
			if _, _, err := runSimulate(t, service, tt.args...); err != nil && !strings.Contains(err.Error(), "not implemented") {
				t.Fatalf("simulate error = %v", err)
			}
			got := service.request
			if got.WaypointName != tt.wantName {
				t.Errorf("WaypointName = %q, want %q", got.WaypointName, tt.wantName)
			}
			if (got.Route == nil) != (tt.wantRoute == nil) || (got.Route != nil && *got.Route != *tt.wantRoute) {
				t.Errorf("Route = %+v, want %+v", got.Route, tt.wantRoute)
			}
			if strings.Join(got.Flags.Files, ",") != strings.Join(tt.wantFiles, ",") {
				t.Errorf("Files = %v, want %v", got.Flags.Files, tt.wantFiles)
			}
			if tt.wantFiles != nil && got.Flags.Stdin == nil {
				t.Errorf("Stdin not set")
			}
		})
	}
}

func TestSimulateCommandUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no positional argument", args: nil},
		{name: "two positional arguments", args: []string{"a", "b"}},
		{name: "route without slash", args: []string{"--route", "shop", "waypoint"}},
		{name: "route with three parts", args: []string{"--route", "a/b/c", "waypoint"}},
		{name: "route with uppercase namespace", args: []string{"--route", "A/b", "waypoint"}},
		{name: "route with empty namespace", args: []string{"--route", "/reviews", "waypoint"}},
		{name: "route with empty name", args: []string{"--route", "shop/", "waypoint"}},
		{name: "unsupported output", args: []string{"--output", "csv", "waypoint"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeSimulateService{status: simulate.StatusFound}
			_, _, err := runSimulate(t, service, tt.args...)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("error = %v, want ErrUsage", err)
			}
		})
	}
}

func TestSimulateCommandServiceErrors(t *testing.T) {
	serviceErr := errors.New("boom")
	tests := []struct {
		name    string
		service *fakeSimulateService
		is      error
		isUsage bool
	}{
		{name: "service error propagates", service: &fakeSimulateService{err: serviceErr}, is: serviceErr},
		{name: "not a waypoint is usage", service: &fakeSimulateService{err: simulate.ErrNotWaypoint}, isUsage: true},
		{name: "waypoint not found maps to gateway not found", service: &fakeSimulateService{status: simulate.StatusWaypointNotFound}, is: ErrGatewayNotFound},
		{name: "route not found maps to ErrRouteNotFound", service: &fakeSimulateService{status: simulate.StatusRouteNotFound}, is: ErrRouteNotFound},
		{name: "route ineligible is usage", service: &fakeSimulateService{status: simulate.StatusRouteIneligible}, isUsage: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, err := runSimulate(t, tt.service, "--route", "shop/reviews", "waypoint")
			if err == nil {
				t.Fatalf("expected error")
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("error = %v, want %v", err, tt.is)
			}
			if tt.isUsage && !errors.Is(err, ErrUsage) {
				t.Errorf("error = %v, want ErrUsage", err)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty (main reports errors)", stderr)
			}
		})
	}
}

func TestSimulateCommandSelectionReasonExplainsFailure(t *testing.T) {
	const reason = "shop/reviews: HTTPRoute is attached to another waypoint"
	service := &fakeSimulateService{
		result: simulate.Result{SelectionReason: reason},
		status: simulate.StatusRouteIneligible,
	}
	_, _, err := runSimulate(t, service, "--route", "shop/reviews", "waypoint")
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != reason {
		t.Errorf("error message = %q, want %q", err.Error(), reason)
	}
	if !errors.Is(err, ErrUsage) || !errors.Is(err, ErrRouteIneligible) {
		t.Errorf("error = %v, want ErrRouteIneligible wrapping ErrUsage", err)
	}
}

func TestSimulateCommandNilServiceFails(t *testing.T) {
	_, _, err := runSimulate(t, nil, "waypoint")
	if err == nil || errors.Is(err, ErrUsage) {
		t.Fatalf("error = %v, want configuration error", err)
	}
}

func TestDefaultSimulateServiceUsesOfflineFiles(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "manifests", "simulate-impact.yaml")
	before := sha256Of(t, fixture)
	service := NewDefaultSimulateService()
	result, status, err := service.Simulate(context.Background(), SimulateRequest{
		WaypointName: "waypoint",
		Route:        &simulate.RouteRef{Namespace: "shop", Name: "primary"},
		Flags: Flags{
			Namespace:  "mesh",
			Files:      []string{fixture},
			Kubeconfig: filepath.Join(t.TempDir(), "must-not-be-read"),
		},
	})
	if err != nil {
		t.Fatalf("Simulate() error = %v", err)
	}
	if status != simulate.StatusFound || result.Kind != simulate.ResultImpact {
		t.Fatalf("Simulate() status = %q kind = %q, want found impact", status, result.Kind)
	}
	if result.Totals.Unrouted != 3 || result.Totals.StillCovered != 3 {
		t.Errorf("totals = %+v, want unrouted 3 and still-covered 3", result.Totals)
	}
	if after := sha256Of(t, fixture); after != before {
		t.Fatalf("fixture hash changed after Simulate()")
	}
}

func TestSimulateTextOutputHasNoColorWhenNotATerminal(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "manifests", "simulate-impact.yaml")
	base := []string{"-f", fixture, "-n", "mesh", "--route", "shop/primary", "waypoint"}
	tests := []struct {
		name    string
		noColor string
		flag    bool
	}{
		{name: "buffer output without NO_COLOR", noColor: ""},
		{name: "--no-color flag", noColor: "", flag: true},
		{name: "NO_COLOR set", noColor: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			args := append([]string{}, base...)
			if tt.flag {
				args = append([]string{"--no-color"}, args...)
			}
			stdout, stderr, err := runSimulate(t, NewDefaultSimulateService(), args...)
			if err != nil {
				t.Fatalf("simulate error = %v, stderr = %q", err, stderr)
			}
			if strings.Contains(stdout, "\x1b[") {
				t.Errorf("stdout contains ANSI escapes:\n%s", stdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty on success", stderr)
			}
		})
	}
}

func TestDefaultSimulateServiceReadsStdin(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "manifests", "simulate-impact.yaml")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	result, status, err := NewDefaultSimulateService().Simulate(context.Background(), SimulateRequest{
		WaypointName: "waypoint",
		Route:        &simulate.RouteRef{Namespace: "shop", Name: "primary"},
		Flags: Flags{
			Namespace: "mesh",
			Files:     []string{"-"},
			Stdin:     strings.NewReader(string(data)),
		},
	})
	if err != nil {
		t.Fatalf("Simulate() error = %v", err)
	}
	if status != simulate.StatusFound || result.Totals.Unrouted != 3 {
		t.Fatalf("Simulate() status = %q totals = %+v, want found with 3 unrouted", status, result.Totals)
	}
}

func TestDefaultSimulateServiceRejectsNamespaceLessManifests(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "manifests", "simulate-missing-namespace.yaml")
	before := sha256Of(t, fixture)
	_, _, err := NewDefaultSimulateService().Simulate(context.Background(), SimulateRequest{
		WaypointName: "waypoint",
		Route:        &simulate.RouteRef{Namespace: "shop", Name: "selected"},
		Flags: Flags{
			Namespace: "mesh",
			Files:     []string{fixture},
		},
	})
	if err == nil {
		t.Fatal("Simulate() error = nil, want missing namespace error")
	}
	if errors.Is(err, ErrUsage) {
		t.Fatalf("error = %v, want plain error (exit 1), not usage", err)
	}
	if !strings.Contains(err.Error(), "missing namespace") {
		t.Errorf("error = %v, want it to mention missing namespace", err)
	}
	if after := sha256Of(t, fixture); after != before {
		t.Fatalf("fixture hash changed after Simulate()")
	}
}

func manifestFixture(name string) string {
	return filepath.Join("..", "..", "testdata", "manifests", name)
}

func TestSimulateWaypointSelectionEndToEnd(t *testing.T) {
	multi := manifestFixture("simulate-multi-waypoint.yaml")
	tests := []struct {
		name      string
		args      []string
		wantUsage bool
		wantErr   string
		wantNil   bool
		wantOut   bool
	}{
		{
			name:      "ingress gateway is rejected as usage with empty stdout",
			args:      []string{"-f", multi, "-n", "edge", "ingress"},
			wantUsage: true,
			wantErr:   "not an Istio ambient waypoint",
		},
		{
			name:    "unknown waypoint is not found",
			args:    []string{"-f", multi, "-n", "mesh", "nope"},
			wantErr: "not found",
		},
		{
			name:      "duplicate waypoint name without namespace is ambiguous usage",
			args:      []string{"-f", multi, "wp-a"},
			wantErr:   "ambiguous",
			wantUsage: true,
		},
		{
			name:    "duplicate waypoint name with namespace resolves",
			args:    []string{"-f", multi, "-n", "mesh", "wp-a"},
			wantNil: true,
			wantOut: true,
		},
		{
			name:    "HBONE-only waypoint resolves with an explicit listing",
			args:    []string{"-f", manifestFixture("gateway-audit-waypoint.yaml"), "-n", "app", "waypoint"},
			wantNil: true,
			wantOut: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := runSimulate(t, NewDefaultSimulateService(), tt.args...)
			if tt.wantNil {
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Fatal("error = nil, want error")
				}
				if errors.Is(err, ErrUsage) != tt.wantUsage {
					t.Errorf("usage = %v, want %v (error: %v)", errors.Is(err, ErrUsage), tt.wantUsage, err)
				}
				if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
				}
			}
			if tt.wantOut && stdout == "" {
				t.Error("stdout is empty, want listing output")
			}
			if !tt.wantOut && stdout != "" {
				t.Errorf("stdout = %q, want empty on error", stdout)
			}
		})
	}
}

func TestSimulateRejectsIstioOnlyManifestAsPlainError(t *testing.T) {
	stdout, _, err := runSimulate(t, NewDefaultSimulateService(), "-f", manifestFixture("istio.yaml"), "waypoint")
	if err == nil {
		t.Fatal("error = nil, want no Gateway API resources error")
	}
	if errors.Is(err, ErrUsage) {
		t.Fatalf("error = %v, want plain error (exit 1), not usage", err)
	}
	if !strings.Contains(err.Error(), "no Gateway API resources were supplied") {
		t.Errorf("error = %v, want it to report missing Gateway API resources", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestDefaultSimulateServiceRequiresNamespaceWithoutKubeconfigDefault(t *testing.T) {
	config := filepath.Join(t.TempDir(), "kubeconfig")
	content := `apiVersion: v1
kind: Config
clusters:
  - name: local
    cluster:
      server: https://127.0.0.1:6443
contexts:
  - name: local
    context:
      cluster: local
      user: local
current-context: local
users:
  - name: local
    user: {}
`
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	_, _, err := NewDefaultSimulateService().Simulate(context.Background(), SimulateRequest{
		WaypointName: "wp-a", Flags: Flags{Kubeconfig: config},
	})
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "--namespace") {
		t.Fatalf("Simulate() error = %v, want namespace usage error", err)
	}
}

// fakeClusterFromSnapshot serves the typed objects of a file snapshot through fake clients.
func fakeClusterFromSnapshot(t *testing.T, snapshot source.Snapshot) *source.ClusterSource {
	t.Helper()
	core := kubefake.NewClientset()
	core.Resources = append(core.Resources, &metav1.APIResourceList{GroupVersion: gatewayv1.GroupVersion.String()})
	var gateways []*gatewayv1.Gateway
	var classes []*gatewayv1.GatewayClass
	var routes []gatewayv1.HTTPRoute
	for _, resource := range snapshot.Resources {
		switch object := resource.Object.(type) {
		case *gatewayv1.Gateway:
			gateways = append(gateways, object)
		case *gatewayv1.GatewayClass:
			classes = append(classes, object)
		case *gatewayv1.HTTPRoute:
			routes = append(routes, *object)
		case *corev1.Namespace:
			if err := core.Tracker().Add(object); err != nil {
				t.Fatalf("seed namespace: %v", err)
			}
		}
	}
	group := gatewayv1.GroupVersion.Group
	gateway := gatewayfake.NewClientset()
	gateway.PrependReactor("get", "gateways", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.GetAction).GetName()
		for _, object := range gateways {
			if object.Namespace == action.GetNamespace() && object.Name == name {
				return true, object, nil
			}
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: group, Resource: "gateways"}, name)
	})
	gateway.PrependReactor("get", "gatewayclasses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.GetAction).GetName()
		for _, object := range classes {
			if object.Name == name {
				return true, object, nil
			}
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: group, Resource: "gatewayclasses"}, name)
	})
	gateway.PrependReactor("list", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &gatewayv1.HTTPRouteList{Items: routes}, nil
	})
	client := kube.NewClientForInterfaces(core, core.Discovery(), nil, gateway.GatewayV1(), gateway.GatewayV1beta1())
	return &source.ClusterSource{Reader: client, Discovery: client.Discovery(), Context: "staging", Namespace: "mesh"}
}

func TestSimulateLiveSnapshotMatchesFileSnapshot(t *testing.T) {
	multi := manifestFixture("simulate-multi-waypoint.yaml")
	snapshot, err := (&source.FilesSource{Files: []string{multi}}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read fixture snapshot: %v", err)
	}
	fileService := defaultSimulateService{}
	liveService := defaultSimulateService{openCluster: func(context.Context, string, string, string) (*source.ClusterSource, error) {
		return fakeClusterFromSnapshot(t, snapshot), nil
	}}
	tests := []struct {
		name  string
		route *simulate.RouteRef
	}{
		{name: "listing"},
		{name: "selected route", route: &simulate.RouteRef{Namespace: "shop", Name: "primary"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			want, wantStatus, err := fileService.Simulate(ctx, SimulateRequest{
				WaypointName: "wp-a", Route: tc.route,
				Flags: Flags{Namespace: "mesh", Files: []string{multi}},
			})
			if err != nil {
				t.Fatalf("file Simulate() error = %v", err)
			}
			got, gotStatus, err := liveService.Simulate(ctx, SimulateRequest{
				WaypointName: "wp-a", Route: tc.route, Flags: Flags{Namespace: "mesh"},
			})
			if err != nil {
				t.Fatalf("live Simulate() error = %v", err)
			}
			if wantStatus != gotStatus {
				t.Fatalf("live status = %q, want %q", gotStatus, wantStatus)
			}
			want.Source, got.Source = model.SourceSnapshot{}, model.SourceSnapshot{}
			// The file-only partial-snapshot disclosure has no live counterpart; everything else must match.
			want.Warnings = slices.DeleteFunc(want.Warnings, func(w string) bool {
				return strings.HasPrefix(w, "local manifests")
			})
			if len(want.Warnings) == 0 {
				want.Warnings = nil
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("live result differs from file result:\nlive: %#v\nfile: %#v", got, want)
			}
		})
	}
}

func TestSimulateLiveWaypointNotFoundStatus(t *testing.T) {
	snapshot := source.Snapshot{}
	service := defaultSimulateService{openCluster: func(context.Context, string, string, string) (*source.ClusterSource, error) {
		return fakeClusterFromSnapshot(t, snapshot), nil
	}}
	_, status, err := service.Simulate(context.Background(), SimulateRequest{
		WaypointName: "missing", Flags: Flags{Namespace: "mesh"},
	})
	if err != nil || status != simulate.StatusWaypointNotFound {
		t.Fatalf("Simulate() = %q, %v; want %q, nil", status, err, simulate.StatusWaypointNotFound)
	}
}

func sha256Of(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return sha256.Sum256(data)
}
