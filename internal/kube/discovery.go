package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
)

func DiscoverGroupVersion(
	ctx context.Context,
	client discovery.DiscoveryInterface,
	groupVersion string,
) (bool, string, error) {
	if err := ctx.Err(); err != nil {
		return false, "", fmt.Errorf("discover API %s: %w", groupVersion, err)
	}
	if client == nil {
		return false, "", fmt.Errorf("discover API %s: discovery client is required", groupVersion)
	}
	_, err := client.ServerResourcesForGroupVersion(groupVersion)
	if err == nil {
		return true, "", nil
	}
	if apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err) || meta.IsNoMatchError(err) {
		return false, err.Error(), nil
	}
	return false, "", fmt.Errorf("discover API %s: %w", groupVersion, err)
}
