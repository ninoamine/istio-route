package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
)

type defaultAnalysisService struct{}

func NewDefaultAnalysisService() AnalysisService {
	return defaultAnalysisService{}
}

func (defaultAnalysisService) Analyze(ctx context.Context, request Request) (model.Analysis, error) {
	if len(request.Flags.Files) > 0 {
		return model.Analysis{}, fmt.Errorf("offline manifest input is not implemented yet")
	}
	cluster, err := source.NewClusterSource(
		ctx, request.Flags.Kubeconfig, request.Flags.Context, request.Flags.Namespace,
	)
	if err != nil {
		return model.Analysis{}, fmt.Errorf("initialize analysis source: %w", err)
	}
	snapshot, err := cluster.Snapshot(ctx)
	if err != nil {
		return model.Analysis{}, fmt.Errorf("read cluster snapshot: %w", err)
	}
	for _, family := range snapshot.Metadata.Families {
		if !family.Available {
			slog.Info("skipping unavailable resource family", "family", family.Family, "reason", family.Reason)
		}
	}
	analysis, err := resolve.Resolve(ctx, request.Hostname, snapshot, cluster.Reader)
	if err != nil {
		return model.Analysis{}, fmt.Errorf("analyze hostname %q: %w", request.Hostname, err)
	}
	if len(analysis.Routes) == 0 {
		return model.Analysis{}, fmt.Errorf("%w: no supported route matches %q", ErrHostnameNotFound, request.Hostname)
	}
	return analysis, nil
}
