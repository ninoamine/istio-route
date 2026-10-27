package resolve

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type endpointTestReader struct {
	service    *corev1.Service
	serviceErr error
	slices     *discoveryv1.EndpointSliceList
	slicesErr  error
	pod        *corev1.Pod
	podErr     error
}

func (r endpointTestReader) GetService(context.Context, string, string, metav1.GetOptions) (*corev1.Service, error) {
	return r.service, r.serviceErr
}

func (r endpointTestReader) ListEndpointSlices(context.Context, string, metav1.ListOptions) (*discoveryv1.EndpointSliceList, error) {
	if r.slices == nil {
		return &discoveryv1.EndpointSliceList{}, r.slicesErr
	}
	return r.slices, r.slicesErr
}

func (r endpointTestReader) GetPod(context.Context, string, string, metav1.GetOptions) (*corev1.Pod, error) {
	return r.pod, r.podErr
}

func TestEndpointResolverAvailabilityAndErrors(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, "api")
	tests := []struct {
		name        string
		reader      endpointTestReader
		nilReader   bool
		wantStatus  model.Availability
		wantErrText string
	}{
		{name: "nil reader", nilReader: true, wantStatus: model.AvailabilityUnavailable},
		{name: "missing service", wantStatus: model.AvailabilityNotFound},
		{name: "not found service", reader: endpointTestReader{serviceErr: notFound}, wantStatus: model.AvailabilityNotFound},
		{name: "service read error", reader: endpointTestReader{serviceErr: errors.New("denied")}, wantErrText: "resolve destination Service shop/api"},
		{name: "endpoint slice read error", reader: endpointTestReader{service: &corev1.Service{}, slicesErr: errors.New("denied")}, wantErrText: "list EndpointSlices for Service shop/api"},
		{name: "missing target pod", reader: endpointTestReader{
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"}},
			slices: &discoveryv1.EndpointSliceList{Items: []discoveryv1.EndpointSlice{{
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.5"},
					TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "api-1"},
				}},
			}}},
			podErr: apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "api-1"),
		}, wantStatus: model.AvailabilityPresent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reader interface {
				GetService(context.Context, string, string, metav1.GetOptions) (*corev1.Service, error)
				ListEndpointSlices(context.Context, string, metav1.ListOptions) (*discoveryv1.EndpointSliceList, error)
				GetPod(context.Context, string, string, metav1.GetOptions) (*corev1.Pod, error)
			} = tt.reader
			if tt.nilReader {
				reader = nil
			}
			service, err := newEndpointResolver(reader).resolve(context.Background(), "shop", "api")
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("resolve() error = %v, want substring %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve() error = %v", err)
			}
			if service.Availability != tt.wantStatus {
				t.Errorf("service availability = %q, want %q", service.Availability, tt.wantStatus)
			}
			if tt.name == "missing target pod" {
				endpoint := service.Endpoints[0]
				if endpoint.Pod == nil || endpoint.Pod.Availability != model.AvailabilityNotFound {
					t.Errorf("endpoint pod = %#v, want not found", endpoint.Pod)
				}
			}
		})
	}
}

func TestEndpointPortsAndPodReadiness(t *testing.T) {
	port := int32(8080)
	slices := &discoveryv1.EndpointSliceList{Items: []discoveryv1.EndpointSlice{{
		Ports: []discoveryv1.EndpointPort{{Port: nil}, {Port: &port}},
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{"10.0.0.6"},
			TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "shop", Name: "api-2"},
		}},
	}}}
	reader := endpointTestReader{
		service: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80}}},
		},
		slices: slices,
		pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-2"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}
	service, err := newEndpointResolver(reader).resolve(context.Background(), "shop", "api")
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if len(service.Endpoints) != 1 || len(service.Endpoints[0].Ports) != 1 {
		t.Fatalf("endpoints = %#v, want one endpoint and one valid port", service.Endpoints)
	}
	endpointPort := service.Endpoints[0].Ports[0]
	if endpointPort.Protocol != string(corev1.ProtocolTCP) || endpointPort.Name != "" {
		t.Errorf("endpoint port = %#v, want default TCP protocol and empty name", endpointPort)
	}
	if service.Endpoints[0].Pod == nil || service.Endpoints[0].Pod.Ready == nil || *service.Endpoints[0].Pod.Ready {
		t.Errorf("pod = %#v, want present but not ready", service.Endpoints[0].Pod)
	}
}
