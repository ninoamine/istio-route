package resolve

import (
	"context"
	"fmt"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type endpointResolver struct {
	reader kube.EndpointReader
	cache  map[string]*model.Service
}

func newEndpointResolver(reader kube.EndpointReader) *endpointResolver {
	return &endpointResolver{reader: reader, cache: make(map[string]*model.Service)}
}

func (r *endpointResolver) resolve(ctx context.Context, namespace, name string) (*model.Service, error) {
	key := namespace + "/" + name
	if service, ok := r.cache[key]; ok {
		return service, nil
	}
	service := &model.Service{
		Identity: model.ResourceIdentity{
			Version: "v1", Kind: "Service", Namespace: namespace, Name: name,
		},
		Availability: model.AvailabilityUnavailable,
	}
	r.cache[key] = service
	if r.reader == nil {
		return service, nil
	}

	kubeService, err := r.reader.GetService(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			service.Availability = model.AvailabilityNotFound
			return service, nil
		}
		return nil, fmt.Errorf("resolve destination Service %s: %w", key, err)
	}
	if kubeService == nil {
		service.Availability = model.AvailabilityNotFound
		return service, nil
	}
	service.Availability = model.AvailabilityPresent
	service.Ports = make([]model.ServicePort, 0, len(kubeService.Spec.Ports))
	for _, port := range kubeService.Spec.Ports {
		service.Ports = append(service.Ports, model.ServicePort{
			Name: port.Name, Port: port.Port, Protocol: string(port.Protocol),
		})
	}

	slices, err := r.reader.ListEndpointSlices(ctx, namespace, metav1.ListOptions{
		LabelSelector: labels.Set{discoveryv1.LabelServiceName: name}.AsSelector().String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list EndpointSlices for Service %s: %w", key, err)
	}
	for _, slice := range slices.Items {
		for _, endpoint := range slice.Endpoints {
			entry := model.Endpoint{
				Addresses:   endpoint.Addresses,
				Ready:       endpoint.Conditions.Ready,
				Serving:     endpoint.Conditions.Serving,
				Terminating: endpoint.Conditions.Terminating,
				Ports:       endpointPorts(slice.Ports),
			}
			if endpoint.TargetRef != nil {
				target := model.ResourceIdentity{
					Version:   "v1",
					Kind:      endpoint.TargetRef.Kind,
					Namespace: endpoint.TargetRef.Namespace,
					Name:      endpoint.TargetRef.Name,
				}
				if target.Namespace == "" {
					target.Namespace = namespace
				}
				entry.Target = &target
				if target.Kind == "Pod" {
					pod, podErr := r.reader.GetPod(ctx, target.Namespace, target.Name, metav1.GetOptions{})
					if podErr != nil && !apierrors.IsNotFound(podErr) {
						return nil, fmt.Errorf("get endpoint Pod %s/%s: %w", target.Namespace, target.Name, podErr)
					}
					if podErr == nil && pod != nil {
						entry.Pod = podModel(pod)
					} else {
						entry.Pod = &model.Pod{
							Identity: target, Availability: model.AvailabilityNotFound,
						}
					}
				}
			}
			service.Endpoints = append(service.Endpoints, entry)
		}
	}
	return service, nil
}

func endpointPorts(ports []discoveryv1.EndpointPort) []model.ServicePort {
	result := make([]model.ServicePort, 0, len(ports))
	for _, port := range ports {
		if port.Port == nil {
			continue
		}
		result = append(result, model.ServicePort{
			Name:     valueOrEmpty(port.Name),
			Port:     *port.Port,
			Protocol: valueOrDefault(port.Protocol, string(corev1.ProtocolTCP)),
		})
	}
	return result
}

func podModel(pod *corev1.Pod) *model.Pod {
	ready := false
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			ready = condition.Status == corev1.ConditionTrue
			break
		}
	}
	return &model.Pod{
		Identity: model.ResourceIdentity{
			Version: "v1", Kind: "Pod",
			Namespace: pod.Namespace, Name: pod.Name,
		},
		Ready:        &ready,
		Phase:        string(pod.Status.Phase),
		Availability: model.AvailabilityPresent,
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func valueOrDefault(value *corev1.Protocol, fallback string) string {
	if value == nil {
		return fallback
	}
	return string(*value)
}
