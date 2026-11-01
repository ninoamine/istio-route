package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/diagnose"
	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
)

type defaultAnalysisService struct{}

func NewDefaultAnalysisService() AnalysisService {
	return defaultAnalysisService{}
}

func (defaultAnalysisService) Analyze(ctx context.Context, request Request) (model.Analysis, error) {
	var analysisSource source.SnapshotSource
	var endpointReader kube.EndpointReader
	if len(request.Flags.Files) > 0 {
		analysisSource = &source.FilesSource{Files: request.Flags.Files, Stdin: request.Flags.Stdin}
	} else {
		cluster, err := source.NewClusterSource(
			ctx, request.Flags.Kubeconfig, request.Flags.Context, request.Flags.Namespace,
		)
		if err != nil {
			return model.Analysis{}, fmt.Errorf("initialize analysis source: %w", err)
		}
		analysisSource = cluster
		endpointReader = cluster.Reader
	}
	snapshot, err := analysisSource.Snapshot(ctx)
	if err != nil {
		return model.Analysis{}, fmt.Errorf("read analysis snapshot: %w", err)
	}
	if endpointReader == nil && len(request.Flags.Files) > 0 {
		endpointReader = resolve.NewSnapshotEndpointReader(snapshot)
	}
	return analyzeSnapshot(ctx, request.Hostname, snapshot, endpointReader)
}

func analyzeSnapshot(
	ctx context.Context,
	hostname string,
	snapshot source.Snapshot,
	endpointReader kube.EndpointReader,
) (model.Analysis, error) {
	analysis, err := resolve.Resolve(ctx, hostname, snapshot, endpointReader)
	if err != nil {
		return model.Analysis{}, fmt.Errorf("analyze hostname %q: %w", hostname, err)
	}
	if err := diagnose.Diagnose(&analysis, snapshot); err != nil {
		return model.Analysis{}, fmt.Errorf("diagnose hostname %q: %w", hostname, err)
	}
	if len(analysis.Routes) == 0 {
		explanations := make([]string, 0, len(analysis.Findings))
		for _, finding := range analysis.Findings {
			if finding.Severity == model.FindingSeverityWarning {
				explanations = append(explanations, finding.Message)
			}
		}
		if len(explanations) == 0 {
			explanations = append(explanations, fmt.Sprintf("no supported route matches %q", hostname))
		}
		return analysis, fmt.Errorf("%w: %s", ErrHostnameNotFound, strings.Join(explanations, "; "))
	}
	return analysis, nil
}
