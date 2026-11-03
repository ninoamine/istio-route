package audit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
)

func orphanedFindings(result Result) []model.Finding {
	var findings []model.Finding
	for _, route := range result.Routes {
		for _, acceptance := range route.Acceptance {
			if acceptance.State != RouteStateRejected {
				continue
			}
			detail := acceptance.Reason
			if acceptance.Message != "" {
				if detail != "" {
					detail += ": "
				}
				detail += acceptance.Message
			}
			message := fmt.Sprintf("%s %s/%s is rejected by Gateway %s/%s",
				route.Identity.Kind, route.Identity.Namespace, route.Identity.Name,
				acceptance.Parent.Identity.Namespace, acceptance.Parent.Identity.Name)
			if detail != "" {
				message += ": " + detail
			}
			findings = append(findings, model.Finding{
				Code: "rejected-route", Severity: model.FindingSeverityError,
				Message: message, Resources: []model.ResourceIdentity{route.Identity},
				Category:     string(CategoryOrphaned),
				SuggestedFix: "Review the route status reason and correct its parent reference or listener constraints.",
			})
		}
		for _, rule := range route.Rules {
			for _, backend := range rule.Backends {
				if backend.Reference.Kind != "" && backend.Reference.Kind != "Service" ||
					backend.Reference.Group != "" {
					continue
				}
				if backend.Service != nil && backend.Service.Availability == model.AvailabilityNotFound {
					findings = append(findings, model.Finding{
						Code: "backend-service-not-found", Severity: model.FindingSeverityError,
						Message: fmt.Sprintf("%s %s/%s rule %d references missing Service %s/%s",
							route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index,
							backend.Reference.Namespace, backend.Reference.Name),
						Resources:    []model.ResourceIdentity{route.Identity, backend.Service.Identity},
						Category:     string(CategoryOrphaned),
						SuggestedFix: "Create the referenced Service or update the route backend reference.",
					})
				} else if backend.Service != nil && backend.Service.Availability == model.AvailabilityPresent &&
					backend.Service.PortPresent != nil && !*backend.Service.PortPresent {
					findings = append(findings, model.Finding{
						Code: "backend-port-not-found", Severity: model.FindingSeverityError,
						Message: fmt.Sprintf("%s %s/%s rule %d references port %d not declared by Service %s/%s",
							route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index,
							derefInt32(backend.Reference.Port), backend.Reference.Namespace, backend.Reference.Name),
						Resources:    []model.ResourceIdentity{route.Identity, backend.Service.Identity},
						Category:     string(CategoryOrphaned),
						SuggestedFix: "Use a port declared by the Service or add the referenced Service port.",
					})
				}
				if backend.Grant != nil && !backend.Grant.Authorized {
					relevantGrants := relevantReferenceGrants(backend.Grant.Grants, route.Identity, backend.Reference)
					resources := []model.ResourceIdentity{route.Identity}
					for _, grant := range relevantGrants {
						resources = append(resources, grant.Identity)
					}
					findings = append(findings, model.Finding{
						Code: "missing-reference-grant", Severity: model.FindingSeverityWarning,
						Message: fmt.Sprintf("cross-namespace backend %s/%s is not authorized by a ReferenceGrant",
							backend.Reference.Namespace, backend.Reference.Name),
						Resources: resources, Category: string(CategoryOrphaned),
						SuggestedFix: "Add a ReferenceGrant in the backend namespace that permits this route to reference the Service.",
					})
				}
			}
		}
	}
	findings = append(findings, unusedGrantFindings(result)...)
	for _, selection := range result.WaypointSelections {
		if !selection.TargetFound {
			findings = append(findings, model.Finding{
				Code: "waypoint-not-found", Severity: model.FindingSeverityError,
				Message: fmt.Sprintf("%s %s/%s selects missing waypoint %s/%s",
					selection.Resource.Kind, selection.Resource.Namespace, selection.Resource.Name,
					selection.WaypointNamespace, selection.WaypointName),
				Resources:    []model.ResourceIdentity{selection.Resource},
				Category:     string(CategoryOrphaned),
				SuggestedFix: "Create the selected waypoint or update the istio.io/use-waypoint label.",
			})
			continue
		}
		if selection.Programmed == nil || selection.Programmed.Status != model.ConditionStatusTrue {
			findings = append(findings, model.Finding{
				Code: "waypoint-not-programmed", Severity: model.FindingSeverityError,
				Message: fmt.Sprintf("waypoint %s/%s selected by %s %s/%s is not programmed",
					selection.WaypointNamespace, selection.WaypointName,
					selection.Resource.Kind, selection.Resource.Namespace, selection.Resource.Name),
				Resources:    []model.ResourceIdentity{selection.Resource},
				Category:     string(CategoryOrphaned),
				SuggestedFix: "Resolve the waypoint Gateway's Programmed condition before selecting it.",
			})
		}
	}
	return findings
}

func relevantReferenceGrants(
	grants []ReferenceGrant,
	route model.ResourceIdentity,
	backend BackendReference,
) []ReferenceGrant {
	var relevant []ReferenceGrant
	for _, grant := range grants {
		fromMatches := false
		for _, from := range grant.From {
			if from.Group == route.Group && from.Kind == route.Kind && from.Namespace == route.Namespace {
				fromMatches = true
				break
			}
		}
		if !fromMatches {
			continue
		}
		for _, to := range grant.To {
			if to.Group == backend.Group && to.Kind == backend.Kind &&
				(to.Name == "" || to.Name == backend.Name) {
				relevant = append(relevant, grant)
				break
			}
		}
	}
	return relevant
}

func unusedGrantFindings(result Result) []model.Finding {
	grants := make(map[string]ReferenceGrant)
	for _, route := range result.Routes {
		for _, rule := range route.Rules {
			for _, backend := range rule.Backends {
				if backend.Grant == nil {
					continue
				}
				for _, grant := range backend.Grant.Grants {
					grants[identityKey(grant.Identity)] = grant
				}
			}
		}
	}
	keys := make([]string, 0, len(grants))
	for key := range grants {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var findings []model.Finding
	for _, key := range keys {
		grant := grants[key]
		relevant, used := false, false
		for _, from := range grant.From {
			for _, route := range result.Routes {
				if from.Group != route.Identity.Group || from.Kind != route.Identity.Kind ||
					from.Namespace != route.Identity.Namespace {
					continue
				}
				for _, to := range grant.To {
					if to.Group != "" || to.Kind != "Service" {
						continue
					}
					relevant = true
					for _, rule := range route.Rules {
						for _, backend := range rule.Backends {
							if backend.Reference.Namespace == grant.Identity.Namespace &&
								backend.Reference.Kind == "Service" &&
								backend.Reference.Group == to.Group &&
								(backend.Reference.Name == to.Name || to.Name == "") {
								used = true
							}
						}
					}
				}
			}
		}
		if relevant && !used {
			findings = append(findings, model.Finding{
				Code: "unused-reference-grant", Severity: model.FindingSeverityWarning,
				Message: fmt.Sprintf("ReferenceGrant %s/%s authorizes no selected route backend",
					grant.Identity.Namespace, grant.Identity.Name),
				Resources:    []model.ResourceIdentity{grant.Identity},
				Category:     string(CategoryOrphaned),
				SuggestedFix: "Remove the unused grant or add a matching cross-namespace backend reference.",
			})
		}
	}
	return findings
}

func derefInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func normalizeResourceList(resources []model.ResourceIdentity) []model.ResourceIdentity {
	result := append([]model.ResourceIdentity(nil), resources...)
	sort.Slice(result, func(i, j int) bool {
		return strings.Compare(identityKey(result[i]), identityKey(result[j])) < 0
	})
	return result
}
