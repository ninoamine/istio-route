package main

import (
	"fmt"
	"os"
	"regexp"

	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"
)

const (
	namespaceCount       = 100
	servicesPerNamespace = 100
	routesPerFamily      = 100
)

var validRunID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,20}$`)

func main() {
	if len(os.Args) != 2 || !validRunID.MatchString(os.Args[1]) {
		fmt.Fprintln(os.Stderr, "usage: generate.go <run-id> (lowercase letters, digits, and hyphens; max 21 characters)")
		os.Exit(2)
	}
	if err := generate(os.Stdout, os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "generate performance workload: %v\n", err)
		os.Exit(1)
	}
}

func generate(output *os.File, runID string) error {
	host := "perf-" + runID + ".example.test"
	className := "perf-" + runID
	gatewayName := "perf-" + runID
	backendNamespace := performanceNamespace(runID, 0)
	runLabels := map[string]string{
		"app.kubernetes.io/managed-by": "istio-route-performance",
		"istio-route.dev/run-id":       runID,
	}
	for namespaceIndex := 0; namespaceIndex < namespaceCount; namespaceIndex++ {
		namespace := performanceNamespace(runID, namespaceIndex)
		if err := emit(output, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: runLabels},
		}); err != nil {
			return err
		}
		for serviceIndex := 0; serviceIndex < servicesPerNamespace; serviceIndex++ {
			name := fmt.Sprintf("svc-%04d", serviceIndex)
			if namespaceIndex == 0 && serviceIndex == 0 {
				name = "backend-a"
			} else if namespaceIndex == 0 && serviceIndex == 1 {
				name = "backend-b"
			}
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace, Name: name, Labels: runLabels,
				},
				Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
					Name: "http", Port: 8080, TargetPort: intstr.FromInt32(8080),
				}}},
			}
			if namespaceIndex == 0 && serviceIndex < 2 {
				podPrefix := "backend-a"
				if serviceIndex == 1 {
					podPrefix = "backend-b"
				}
				service.Spec.Selector = map[string]string{"app": podPrefix}
			}
			if err := emit(output, service); err != nil {
				return err
			}
			if namespaceIndex == 0 && serviceIndex < 2 {
				if err := emitBackendWorkloads(output, namespace, name, runLabels); err != nil {
					return err
				}
			}
		}
	}

	istioGateway := &istioclientv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: backendNamespace, Name: gatewayName, Labels: runLabels},
		Spec: istiov1alpha3.Gateway{Servers: []*istiov1alpha3.Server{{
			Name:  "http",
			Port:  &istiov1alpha3.Port{Number: 80, Name: "http", Protocol: "HTTP"},
			Hosts: []string{host},
		}}},
	}
	if err := emit(output, istioGateway); err != nil {
		return err
	}
	for routeIndex := 0; routeIndex < routesPerFamily; routeIndex++ {
		routeHost := host
		if routeIndex != 0 {
			routeHost = fmt.Sprintf("decoy-%03d.%s.invalid", routeIndex, runID)
		}
		virtualService := &istioclientv1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: backendNamespace, Name: fmt.Sprintf("vs-%03d", routeIndex), Labels: runLabels,
			},
			Spec: istiov1alpha3.VirtualService{
				Hosts:    []string{routeHost},
				Gateways: []string{gatewayName},
				Http: []*istiov1alpha3.HTTPRoute{{
					Route: []*istiov1alpha3.HTTPRouteDestination{
						performanceIstioDestination("backend-a", backendNamespace, 50),
						performanceIstioDestination("backend-b", backendNamespace, 50),
					},
				}},
			},
		}
		if err := emit(output, virtualService); err != nil {
			return err
		}
	}

	if err := emit(output, &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: className, Labels: runLabels},
		Spec:       gatewayv1.GatewayClassSpec{ControllerName: "istio.io/gateway-controller"},
	}); err != nil {
		return err
	}
	if err := emit(output, &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: backendNamespace, Name: gatewayName, Labels: runLabels},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(className),
			Listeners: []gatewayv1.Listener{{
				Name: "http", Hostname: hostnamePointer(host),
				Port: 80, Protocol: gatewayv1.HTTPProtocolType,
				AllowedRoutes: &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{
					From: routeNamespacePointer(gatewayv1.NamespacesFromSame),
				}},
			}},
		},
	}); err != nil {
		return err
	}
	for routeIndex := 0; routeIndex < routesPerFamily; routeIndex++ {
		routeHost := gatewayv1.Hostname(host)
		if routeIndex != 0 {
			routeHost = gatewayv1.Hostname(fmt.Sprintf("decoy-%03d.%s.invalid", routeIndex, runID))
		}
		route := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: backendNamespace, Name: fmt.Sprintf("route-%03d", routeIndex), Labels: runLabels,
			},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(gatewayName)}}},
				Hostnames:       []gatewayv1.Hostname{routeHost},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						performanceGatewayBackend("backend-a", 50),
						performanceGatewayBackend("backend-b", 50),
					},
				}},
			},
		}
		if err := emit(output, route); err != nil {
			return err
		}
	}
	return nil
}

func emitBackendWorkloads(
	output *os.File,
	namespace, serviceName string,
	runLabels map[string]string,
) error {
	for endpointIndex := 0; endpointIndex < 3; endpointIndex++ {
		podName := fmt.Sprintf("%s-%d", serviceName, endpointIndex)
		podLabels := map[string]string{"app": serviceName}
		for key, value := range runLabels {
			podLabels[key] = value
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName, Labels: podLabels},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "backend", Image: "registry.k8s.io/pause:3.10"}}},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}
		if err := emit(output, pod); err != nil {
			return err
		}
	}
	return nil
}

func performanceIstioDestination(name, namespace string, weight int32) *istiov1alpha3.HTTPRouteDestination {
	return &istiov1alpha3.HTTPRouteDestination{
		Destination: &istiov1alpha3.Destination{
			Host: name + "." + namespace + ".svc.cluster.local",
			Port: &istiov1alpha3.PortSelector{Number: 8080},
		},
		Weight: weight,
	}
}

func performanceGatewayBackend(name string, weight int32) gatewayv1.HTTPBackendRef {
	return gatewayv1.HTTPBackendRef{BackendRef: gatewayv1.BackendRef{
		BackendObjectReference: gatewayv1.BackendObjectReference{
			Name: gatewayv1.ObjectName(name), Port: portPointer(8080),
		},
		Weight: int32Pointer(weight),
	}}
}

func emit(output *os.File, object runtime.Object) error {
	switch typed := object.(type) {
	case *corev1.Namespace:
		typed.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
	case *corev1.Service:
		typed.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Service"))
	case *corev1.Pod:
		typed.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))
	case *istioclientv1.Gateway:
		typed.SetGroupVersionKind(istioclientv1.SchemeGroupVersion.WithKind("Gateway"))
	case *istioclientv1.VirtualService:
		typed.SetGroupVersionKind(istioclientv1.SchemeGroupVersion.WithKind("VirtualService"))
	case *gatewayv1.GatewayClass:
		typed.SetGroupVersionKind(schema.GroupVersion{Group: gatewayv1.GroupName, Version: "v1"}.WithKind("GatewayClass"))
	case *gatewayv1.Gateway:
		typed.SetGroupVersionKind(schema.GroupVersion{Group: gatewayv1.GroupName, Version: "v1"}.WithKind("Gateway"))
	case *gatewayv1.HTTPRoute:
		typed.SetGroupVersionKind(schema.GroupVersion{Group: gatewayv1.GroupName, Version: "v1"}.WithKind("HTTPRoute"))
	default:
		return fmt.Errorf("unsupported object type %T", object)
	}
	encoded, err := yaml.Marshal(object)
	if err != nil {
		return fmt.Errorf("marshal %T: %w", object, err)
	}
	if _, err := fmt.Fprintf(output, "---\n%s", encoded); err != nil {
		return fmt.Errorf("write %T: %w", object, err)
	}
	return nil
}

func performanceNamespace(runID string, index int) string {
	return fmt.Sprintf("istio-route-perf-%s-%02d", runID, index)
}

func hostnamePointer(value string) *gatewayv1.Hostname {
	hostname := gatewayv1.Hostname(value)
	return &hostname
}

func routeNamespacePointer(value gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces {
	return &value
}

func portPointer(value int32) *gatewayv1.PortNumber {
	port := gatewayv1.PortNumber(value)
	return &port
}

func int32Pointer(value int32) *int32 {
	return &value
}
