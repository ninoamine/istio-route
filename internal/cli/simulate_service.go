package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/simulate"
	"github.com/ninoamine/istio-route/internal/source"
)

// clusterOpener builds the cluster source for live simulation; tests substitute a fake.
type clusterOpener func(ctx context.Context, kubeconfig, contextName, namespace string) (*source.ClusterSource, error)

type defaultSimulateService struct {
	openCluster clusterOpener
}

func NewDefaultSimulateService() SimulateService {
	return defaultSimulateService{openCluster: source.NewClusterSource}
}

// Simulate reads manifests when files are given, otherwise a live cluster snapshot.
func (s defaultSimulateService) Simulate(
	ctx context.Context,
	request SimulateRequest,
) (simulate.Result, simulate.Status, error) {
	var snapshot source.Snapshot
	if len(request.Flags.Files) > 0 {
		fileSnapshot, err := (&source.FilesSource{Files: request.Flags.Files, Stdin: request.Flags.Stdin}).Snapshot(ctx)
		if err != nil {
			return simulate.Result{}, "", fmt.Errorf("read simulation snapshot: %w", err)
		}
		snapshot = fileSnapshot
	} else {
		open := s.openCluster
		if open == nil {
			open = source.NewClusterSource
		}
		cluster, err := open(
			ctx, request.Flags.Kubeconfig, request.Flags.Context, request.Flags.Namespace,
		)
		if err != nil {
			return simulate.Result{}, "", fmt.Errorf("initialize simulation source: %w", err)
		}
		if cluster.Namespace == "" {
			return simulate.Result{}, "", fmt.Errorf("%w: --namespace is required when the kubeconfig has no default namespace", ErrUsage)
		}
		var routeNamespace, routeName string
		if request.Route != nil {
			routeNamespace, routeName = request.Route.Namespace, request.Route.Name
		}
		clusterSnapshot, err := cluster.WaypointSnapshot(ctx, request.WaypointName, cluster.Namespace, routeNamespace, routeName)
		if err != nil {
			if errors.Is(err, source.ErrGatewayNotFound) {
				return simulate.Result{}, simulate.StatusWaypointNotFound, nil
			}
			return simulate.Result{}, "", fmt.Errorf("read simulation snapshot: %w", err)
		}
		snapshot = clusterSnapshot
	}
	result, status, err := simulate.Run(ctx, simulate.Request{
		WaypointName: request.WaypointName,
		Namespace:    request.Flags.Namespace,
		Route:        request.Route,
	}, snapshot)
	if err != nil {
		if errors.Is(err, audit.ErrAmbiguousGateway) {
			return simulate.Result{}, "", fmt.Errorf("%w: %w", ErrUsage, err)
		}
		return simulate.Result{}, "", fmt.Errorf("simulate %q: %w", request.WaypointName, err)
	}
	return result, status, nil
}
