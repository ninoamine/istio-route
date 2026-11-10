package simulate

import (
	"context"
	"errors"
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
)

// Request selects the waypoint and, optionally, the HTTPRoute whose removal is simulated.
type Request struct {
	WaypointName string
	Namespace    string
	Route        *RouteRef
}

// Run evaluates a simulation request against an in-memory snapshot. It never mutates snapshot.
// Route selection failures return a listing together with a non-OK status and a nil error so the
// caller can render the listing before choosing an exit code.
func Run(ctx context.Context, request Request, snapshot source.Snapshot) (Result, Status, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, "", fmt.Errorf("simulate: %w", err)
	}

	scope := Scope{DirectHTTPRouteOnly: true}
	var warnings []string
	if snapshot.Metadata.Mode == model.SourceModeFiles {
		if err := ValidateLocalNamespaces(snapshot, request.WaypointName, request.Route); err != nil {
			return Result{}, "", err
		}
		if err := RequireGatewayAPI(snapshot); err != nil {
			return Result{}, "", err
		}
		warnings = append(warnings,
			"local manifests are a partial view: resources not supplied in the input files are not considered")
	}

	waypoint, err := ResolveWaypoint(snapshot, request.WaypointName, request.Namespace)
	if err != nil {
		if errors.Is(err, ErrWaypointNotFound) {
			return Result{}, StatusWaypointNotFound, nil
		}
		return Result{}, "", fmt.Errorf("resolve waypoint: %w", err)
	}

	attachments := EvaluateAttachments(snapshot, waypoint)
	scope.ServiceParentedExcluded = len(ServiceParentedExclusions(snapshot))

	result := Result{Source: snapshot.Metadata, Waypoint: waypoint, Scope: scope, Warnings: warnings}
	if request.Route == nil {
		result.Kind = ResultListing
		result.Routes = EligibleRoutes(attachments)
		return result, StatusFound, nil
	}

	target := *request.Route
	if _, err := SelectRoute(snapshot, attachments, target); err != nil {
		result.Kind = ResultListing
		result.Routes = EligibleRoutes(attachments)
		result.SelectionReason = err.Error()
		switch {
		case errors.Is(err, ErrRouteNotFound):
			return result, StatusRouteNotFound, nil
		case errors.Is(err, ErrRouteIneligible):
			return result, StatusRouteIneligible, nil
		default:
			return Result{}, "", fmt.Errorf("select route: %w", err)
		}
	}

	scope.OtherParentNotes = otherParentNotes(snapshot, target, waypoint)
	warnings = append(warnings, scope.OtherParentNotes...)

	before := BuildView(snapshot, waypoint, attachments)
	afterAttachments := withoutRoute(attachments, target)
	after := BuildView(Without(snapshot, target), waypoint, afterAttachments)

	impact := BuildImpact(before, after, target, waypoint, scope)
	impact.Source = snapshot.Metadata
	impact.Warnings = append(warnings, impact.Warnings...)
	impact.SelectedRoute = &target
	return impact, StatusFound, nil
}

func withoutRoute(attachments []RouteAttachments, target RouteRef) []RouteAttachments {
	kept := make([]RouteAttachments, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Route != target {
			kept = append(kept, attachment)
		}
	}
	return kept
}
