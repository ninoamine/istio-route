package simulate

import (
	"errors"
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
)

const waypointClassName = "istio-waypoint"

// Classification is the outcome of one traffic region for one target rule.
type Classification string

const (
	ClassificationUnrouted     Classification = "unrouted"
	ClassificationStillCovered Classification = "still-covered"
	ClassificationNoChange     Classification = "no-change"
	ClassificationUndetermined Classification = "undetermined"
)

func (c Classification) Validate() error {
	switch c {
	case ClassificationUnrouted, ClassificationStillCovered, ClassificationNoChange, ClassificationUndetermined:
		return nil
	default:
		return fmt.Errorf("unsupported outcome %q", c)
	}
}

// AttachmentState is the evidence state of one route on one waypoint listener.
type AttachmentState string

const (
	AttachmentAccepted    AttachmentState = "accepted"
	AttachmentRejected    AttachmentState = "rejected"
	AttachmentUnconfirmed AttachmentState = "unconfirmed"
)

func (s AttachmentState) Validate() error {
	switch s {
	case AttachmentAccepted, AttachmentRejected, AttachmentUnconfirmed:
		return nil
	default:
		return fmt.Errorf("unsupported attachment state %q", s)
	}
}

// ValidationState reports whether the waypoint's GatewayClass was confirmed in the snapshot.
type ValidationState string

const (
	ValidationConfirmed   ValidationState = "confirmed"
	ValidationUnconfirmed ValidationState = "unconfirmed"
)

func (s ValidationState) Validate() error {
	switch s {
	case ValidationConfirmed, ValidationUnconfirmed:
		return nil
	default:
		return fmt.Errorf("unsupported validation state %q", s)
	}
}

// ResultKind distinguishes an eligible-route listing from an impact report.
type ResultKind string

const (
	ResultListing ResultKind = "listing"
	ResultImpact  ResultKind = "impact"
)

func (k ResultKind) Validate() error {
	switch k {
	case ResultListing, ResultImpact:
		return nil
	default:
		return fmt.Errorf("unsupported result kind %q", k)
	}
}

// Status is the selection outcome of a simulation request; it is not a classification.
type Status string

const (
	StatusFound            Status = "found"
	StatusWaypointNotFound Status = "waypoint-not-found"
	StatusRouteNotFound    Status = "route-not-found"
	StatusRouteIneligible  Status = "route-ineligible"
)

// RouteRef identifies an HTTPRoute by namespace and name. The group and kind are implied.
type RouteRef struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
}

func (r RouteRef) Validate() error {
	if r.Namespace == "" {
		return errors.New("route namespace is required")
	}
	if r.Name == "" {
		return errors.New("route name is required")
	}
	return nil
}

// RuleRef identifies a target rule by route identity and 1-based position.
type RuleRef struct {
	Route RouteRef `json:"route" yaml:"route"`
	Index int      `json:"index" yaml:"index"`
}

func (r RuleRef) Validate() error {
	if err := r.Route.Validate(); err != nil {
		return err
	}
	if r.Index < 1 {
		return errors.New("rule index must be greater than zero")
	}
	return nil
}

// Waypoint is the selected ambient waypoint Gateway.
type Waypoint struct {
	Namespace        string             `json:"namespace" yaml:"namespace"`
	Name             string             `json:"name" yaml:"name"`
	Class            string             `json:"class" yaml:"class"`
	Validation       ValidationState    `json:"validation" yaml:"validation"`
	ValidationReason string             `json:"validationReason,omitempty" yaml:"validationReason,omitempty"`
	Listeners        []WaypointListener `json:"listeners" yaml:"listeners"`
}

func (w Waypoint) Validate() error {
	if w.Name == "" {
		return errors.New("waypoint name is required")
	}
	if err := w.Validation.Validate(); err != nil {
		return err
	}
	for i, listener := range w.Listeners {
		if err := listener.Validate(); err != nil {
			return fmt.Errorf("listener %d: %w", i, err)
		}
	}
	return nil
}

// WaypointListener is an applicable waypoint listener (HBONE, HTTP, or HTTPS).
type WaypointListener struct {
	Name     string `json:"name" yaml:"name"`
	Port     int32  `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
}

func (l WaypointListener) Validate() error {
	if l.Name == "" {
		return errors.New("listener name is required")
	}
	if l.Protocol == "" {
		return errors.New("listener protocol is required")
	}
	return nil
}

// ListenerAttachment is the evidence for one route on one applicable listener.
type ListenerAttachment struct {
	Route       RouteRef        `json:"route" yaml:"route"`
	Listener    string          `json:"listener" yaml:"listener"`
	SectionName string          `json:"sectionName,omitempty" yaml:"sectionName,omitempty"`
	Port        *int32          `json:"port,omitempty" yaml:"port,omitempty"`
	State       AttachmentState `json:"state" yaml:"state"`
	Reason      string          `json:"reason,omitempty" yaml:"reason,omitempty"`
}

func (a ListenerAttachment) Validate() error {
	if err := a.Route.Validate(); err != nil {
		return err
	}
	if a.Listener == "" {
		return errors.New("listener name is required")
	}
	return a.State.Validate()
}

// RouteListing is one eligible route in a listing.
type RouteListing struct {
	Route     RouteRef             `json:"route" yaml:"route"`
	Hostnames []string             `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
	Listeners []ListenerAttachment `json:"listeners" yaml:"listeners"`
}

// Predicate describes a traffic region. Exactly one of All, Any, Not, or Atom is set.
type Predicate struct {
	All  []Predicate `json:"all,omitempty" yaml:"all,omitempty"`
	Any  []Predicate `json:"any,omitempty" yaml:"any,omitempty"`
	Not  *Predicate  `json:"not,omitempty" yaml:"not,omitempty"`
	Atom *Atom       `json:"atom,omitempty" yaml:"atom,omitempty"`
}

func (p Predicate) Validate() error {
	set := 0
	if len(p.All) > 0 {
		set++
	}
	if len(p.Any) > 0 {
		set++
	}
	if p.Not != nil {
		set++
	}
	if p.Atom != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("predicate must set exactly one of all, any, not, or atom")
	}
	for i, child := range append(append([]Predicate{}, p.All...), p.Any...) {
		if err := child.Validate(); err != nil {
			return fmt.Errorf("predicate child %d: %w", i, err)
		}
	}
	if p.Not != nil {
		if err := p.Not.Validate(); err != nil {
			return fmt.Errorf("predicate not: %w", err)
		}
	}
	if p.Atom != nil {
		return p.Atom.Validate()
	}
	return nil
}

// Atom is one dimension constraint: hostname, path, method, header, or query.
type Atom struct {
	Dimension string `json:"dimension" yaml:"dimension"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	MatchType string `json:"matchType,omitempty" yaml:"matchType,omitempty"`
	Value     string `json:"value,omitempty" yaml:"value,omitempty"`
}

func (a Atom) Validate() error {
	switch a.Dimension {
	case "hostname", "path", "method":
	case "header", "query":
		if a.Name == "" {
			return fmt.Errorf("%s atom requires a name", a.Dimension)
		}
	default:
		return fmt.Errorf("unsupported predicate dimension %q", a.Dimension)
	}
	return nil
}

// BackendRef is a backend Service reference; there are no health fields.
type BackendRef struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
	Port      *int32 `json:"port,omitempty" yaml:"port,omitempty"`
	Weight    *int32 `json:"weight,omitempty" yaml:"weight,omitempty"`
}

// Provider is the rule that serves a region, with its referenced backends.
type Provider struct {
	Route     RouteRef     `json:"route" yaml:"route"`
	RuleIndex int          `json:"ruleIndex" yaml:"ruleIndex"`
	Backends  []BackendRef `json:"backends,omitempty" yaml:"backends,omitempty"`
}

func (p Provider) Validate() error {
	if err := p.Route.Validate(); err != nil {
		return err
	}
	if p.RuleIndex < 1 {
		return errors.New("provider rule index must be greater than zero")
	}
	return nil
}

// ProviderView states whether the serving rule is known, provably absent, or unknown.
type ProviderView struct {
	State    string    `json:"state" yaml:"state"`
	Provider *Provider `json:"provider,omitempty" yaml:"provider,omitempty"`
}

const (
	ProviderKnown   = "provider"
	ProviderNone    = "none"
	ProviderUnknown = "unknown"
)

func (v ProviderView) Validate() error {
	switch v.State {
	case ProviderNone, ProviderUnknown:
		if v.Provider != nil {
			return fmt.Errorf("provider state %q must not carry a provider", v.State)
		}
		return nil
	case ProviderKnown:
		if v.Provider == nil {
			return errors.New("provider state \"provider\" requires a provider")
		}
		return v.Provider.Validate()
	default:
		return fmt.Errorf("unsupported provider state %q", v.State)
	}
}

// Provisional is a clearly labeled, hypothetical outcome that depends on incomplete evidence.
type Provisional struct {
	Outcome Classification `json:"outcome" yaml:"outcome"`
	Reasons []string       `json:"reasons,omitempty" yaml:"reasons,omitempty"`
}

// Entry is one impact entry: a listener, a target rule, and a traffic region with one outcome.
type Entry struct {
	Listener       string         `json:"listener" yaml:"listener"`
	Rule           RuleRef        `json:"rule" yaml:"rule"`
	Region         Predicate      `json:"region" yaml:"region"`
	Outcome        Classification `json:"outcome" yaml:"outcome"`
	Before         ProviderView   `json:"before" yaml:"before"`
	After          ProviderView   `json:"after" yaml:"after"`
	BackendChanged bool           `json:"backendChanged,omitempty" yaml:"backendChanged,omitempty"`
	Reasons        []string       `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	Provisional    *Provisional   `json:"provisional,omitempty" yaml:"provisional,omitempty"`
}

func (e Entry) Validate() error {
	if e.Listener == "" {
		return errors.New("entry listener is required")
	}
	if err := e.Rule.Validate(); err != nil {
		return fmt.Errorf("entry rule: %w", err)
	}
	if err := e.Region.Validate(); err != nil {
		return fmt.Errorf("entry region: %w", err)
	}
	if err := e.Outcome.Validate(); err != nil {
		return err
	}
	if err := e.Before.Validate(); err != nil {
		return fmt.Errorf("entry before: %w", err)
	}
	if err := e.After.Validate(); err != nil {
		return fmt.Errorf("entry after: %w", err)
	}
	return nil
}

// RuleSummary aggregates the outcomes of one target rule. A rule may have several outcomes.
type RuleSummary struct {
	Rule     RuleRef                `json:"rule" yaml:"rule"`
	Backends []BackendRef           `json:"backends,omitempty" yaml:"backends,omitempty"`
	Outcomes []Classification       `json:"outcomes" yaml:"outcomes"`
	Counts   map[Classification]int `json:"counts" yaml:"counts"`
	Partial  bool                   `json:"partial" yaml:"partial"`
}

func (s RuleSummary) Validate() error {
	if err := s.Rule.Validate(); err != nil {
		return err
	}
	for _, outcome := range s.Outcomes {
		if err := outcome.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Totals counts entries by outcome.
type Totals struct {
	Unrouted     int `json:"unrouted" yaml:"unrouted"`
	StillCovered int `json:"still-covered" yaml:"still-covered"`
	NoChange     int `json:"no-change" yaml:"no-change"`
	Undetermined int `json:"undetermined" yaml:"undetermined"`
}

// Scope discloses what the simulation covers and what it excludes.
type Scope struct {
	DirectHTTPRouteOnly     bool       `json:"directHTTPRouteOnly" yaml:"directHTTPRouteOnly"`
	ServiceParentedExcluded int        `json:"serviceParentedExcluded" yaml:"serviceParentedExcluded"`
	ExcludedRoutes          []RouteRef `json:"excludedRoutes,omitempty" yaml:"excludedRoutes,omitempty"`
	OtherParentNotes        []string   `json:"otherParentNotes,omitempty" yaml:"otherParentNotes,omitempty"`
}

// Result is the logical output of one simulation request.
type Result struct {
	Kind          ResultKind           `json:"kind" yaml:"kind"`
	Source        model.SourceSnapshot `json:"source" yaml:"source"`
	Waypoint      Waypoint             `json:"waypoint" yaml:"waypoint"`
	Scope         Scope                `json:"scope" yaml:"scope"`
	Routes        []RouteListing       `json:"routes,omitempty" yaml:"routes,omitempty"`
	SelectedRoute *RouteRef            `json:"selectedRoute,omitempty" yaml:"selectedRoute,omitempty"`
	// SelectionReason explains a route-selection failure alongside a listing result.
	SelectionReason string        `json:"selectionReason,omitempty" yaml:"selectionReason,omitempty"`
	Entries         []Entry       `json:"entries,omitempty" yaml:"entries,omitempty"`
	RuleSummaries   []RuleSummary `json:"ruleSummaries,omitempty" yaml:"ruleSummaries,omitempty"`
	Totals          Totals        `json:"totals" yaml:"totals"`
	Warnings        []string      `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

func (r Result) Validate() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if err := r.Waypoint.Validate(); err != nil {
		return fmt.Errorf("waypoint: %w", err)
	}
	for i, entry := range r.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
	}
	for i, summary := range r.RuleSummaries {
		if err := summary.Validate(); err != nil {
			return fmt.Errorf("rule summary %d: %w", i, err)
		}
	}
	return nil
}
