package resolve

import (
	"context"
	"fmt"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/source"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type snapshotEndpointReader struct {
	services map[string]*corev1.Service
	slices   map[string][]*discoveryv1.EndpointSlice
	pods     map[string]*corev1.Pod
}

func NewSnapshotEndpointReader(snapshot source.Snapshot) kube.EndpointReader {
	reader := &snapshotEndpointReader{
		services: make(map[string]*corev1.Service),
		slices:   make(map[string][]*discoveryv1.EndpointSlice),
		pods:     make(map[string]*corev1.Pod),
	}
	for _, resource := range snapshot.Resources {
		switch object := resource.Object.(type) {
		case *corev1.Service:
			reader.services[resourceKey(object.Namespace, object.Name)] = object
		case *discoveryv1.EndpointSlice:
			reader.slices[object.Namespace] = append(reader.slices[object.Namespace], object)
		case *corev1.Pod:
			reader.pods[resourceKey(object.Namespace, object.Name)] = object
		}
	}
	return reader
}

func (r *snapshotEndpointReader) GetService(
	ctx context.Context,
	namespace, name string,
	_ metav1.GetOptions,
) (*corev1.Service, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read offline Service: %w", err)
	}
	service := r.services[resourceKey(namespace, name)]
	if service == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, name)
	}
	return service, nil
}

func (r *snapshotEndpointReader) ListEndpointSlices(
	ctx context.Context,
	namespace string,
	options metav1.ListOptions,
) (*discoveryv1.EndpointSliceList, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read offline EndpointSlices: %w", err)
	}
	selector := labels.Everything()
	if options.LabelSelector != "" {
		parsed, err := labels.Parse(options.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parse offline EndpointSlice selector: %w", err)
		}
		selector = parsed
	}
	result := &discoveryv1.EndpointSliceList{}
	for _, slice := range r.slices[namespace] {
		if selector.Matches(labels.Set(slice.Labels)) {
			result.Items = append(result.Items, *slice)
		}
	}
	return result, nil
}

func (r *snapshotEndpointReader) GetPod(
	ctx context.Context,
	namespace, name string,
	_ metav1.GetOptions,
) (*corev1.Pod, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read offline Pod: %w", err)
	}
	pod := r.pods[resourceKey(namespace, name)]
	if pod == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
	}
	return pod, nil
}

func resourceKey(namespace, name string) string {
	return namespace + "/" + name
}
