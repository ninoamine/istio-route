package audit

import (
	"errors"
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

var ErrAmbiguousGateway = errors.New("ambiguous Gateway")

type Category string

const (
	CategoryOrphaned             Category = "orphaned"
	CategoryDuplicated           Category = "duplicated"
	CategoryPrecedence           Category = "precedence"
	CategoryDeficientDestination Category = "deficient-destination"
)

func (c Category) Validate() error {
	switch c {
	case CategoryOrphaned, CategoryDuplicated, CategoryPrecedence, CategoryDeficientDestination:
		return nil
	default:
		return fmt.Errorf("invalid audit finding category %q", c)
	}
}

type RouteState string

const (
	RouteStateAccepted RouteState = "accepted"
	RouteStateRejected RouteState = "rejected"
	RouteStateUnknown  RouteState = "unknown"
)

func (s RouteState) Validate() error {
	switch s {
	case RouteStateAccepted, RouteStateRejected, RouteStateUnknown:
		return nil
	default:
		return fmt.Errorf("invalid route state %q", s)
	}
}

type PrecedenceState string

const (
	PrecedenceWinner         PrecedenceState = "winner"
	PrecedencePartialOverlap PrecedenceState = "partial-overlap"
	PrecedenceShadowed       PrecedenceState = "shadowed"
)

func (s PrecedenceState) Validate() error {
	switch s {
	case PrecedenceWinner, PrecedencePartialOverlap, PrecedenceShadowed:
		return nil
	default:
		return fmt.Errorf("invalid precedence state %q", s)
	}
}

type EndpointCheckState string

const (
	EndpointCheckChecked     EndpointCheckState = "checked"
	EndpointCheckSkipped     EndpointCheckState = "skipped"
	EndpointCheckUnavailable EndpointCheckState = "unavailable"
)

func (s EndpointCheckState) Validate() error {
	switch s {
	case EndpointCheckChecked, EndpointCheckSkipped, EndpointCheckUnavailable:
		return nil
	default:
		return fmt.Errorf("invalid endpoint-check state %q", s)
	}
}

type Outcome string

const (
	OutcomeTargetFound    Outcome = "target-found"
	OutcomeTargetNotFound Outcome = "target-not-found"
)

func (o Outcome) Validate() error {
	switch o {
	case OutcomeTargetFound, OutcomeTargetNotFound:
		return nil
	default:
		return fmt.Errorf("invalid audit outcome %q", o)
	}
}

type Request struct {
	GatewayName string               `json:"gatewayName" yaml:"gatewayName"`
	Namespace   string               `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Source      model.SourceSnapshot `json:"source" yaml:"source"`
}

func (r Request) Validate() error {
	if r.GatewayName == "" {
		return fmt.Errorf("gateway name is required")
	}
	if problems := validation.IsDNS1123Subdomain(r.GatewayName); len(problems) > 0 {
		return fmt.Errorf("invalid gateway name %q: %s", r.GatewayName, problems[0])
	}
	if r.Namespace != "" {
		if problems := validation.IsDNS1123Label(r.Namespace); len(problems) > 0 {
			return fmt.Errorf("invalid gateway namespace %q: %s", r.Namespace, problems[0])
		}
	}
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	return nil
}

type Result struct {
	Source             model.SourceSnapshot `json:"source" yaml:"source"`
	Gateway            Gateway              `json:"gateway" yaml:"gateway"`
	Listeners          []Listener           `json:"listeners,omitempty" yaml:"listeners,omitempty"`
	Routes             []Route              `json:"routes,omitempty" yaml:"routes,omitempty"`
	WaypointSelections []WaypointSelection  `json:"waypointSelections,omitempty" yaml:"waypointSelections,omitempty"`
	Findings           []model.Finding      `json:"findings,omitempty" yaml:"findings,omitempty"`
	EndpointCheck      EndpointCheck        `json:"endpointCheck" yaml:"endpointCheck"`
}

func (r Result) Validate() error {
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if err := r.Gateway.Validate(); err != nil {
		return fmt.Errorf("gateway identity: %w", err)
	}
	if err := r.EndpointCheck.Validate(); err != nil {
		return fmt.Errorf("endpoint check: %w", err)
	}
	for i, listener := range r.Listeners {
		if err := listener.Validate(); err != nil {
			return fmt.Errorf("listener %d: %w", i, err)
		}
	}
	for i, route := range r.Routes {
		if err := route.Validate(); err != nil {
			return fmt.Errorf("route %d: %w", i, err)
		}
	}
	for i, selection := range r.WaypointSelections {
		if err := selection.Validate(); err != nil {
			return fmt.Errorf("waypoint selection %d: %w", i, err)
		}
	}
	for i, finding := range r.Findings {
		if err := finding.Validate(); err != nil {
			return fmt.Errorf("finding %d: %w", i, err)
		}
		if err := Category(finding.Category).Validate(); err != nil {
			return fmt.Errorf("finding %d category: %w", i, err)
		}
	}
	return nil
}

type Gateway struct {
	Identity   model.ResourceIdentity `json:"identity" yaml:"identity"`
	Class      model.GatewayClassRef  `json:"class" yaml:"class"`
	Programmed *model.Condition       `json:"programmed,omitempty" yaml:"programmed,omitempty"`
}

func (g Gateway) Validate() error {
	if err := g.Identity.Validate(); err != nil {
		return err
	}
	if g.Identity.Kind != "Gateway" || g.Identity.Namespace == "" {
		return fmt.Errorf("identity must name a namespaced Gateway")
	}
	if err := g.Class.Identity.Validate(); err != nil {
		return fmt.Errorf("GatewayClass: %w", err)
	}
	if g.Class.Identity.Kind != "GatewayClass" {
		return fmt.Errorf("GatewayClass identity kind must be GatewayClass")
	}
	if g.Class.Controller == "" {
		return fmt.Errorf("GatewayClass controller is required")
	}
	if g.Programmed != nil {
		if err := g.Programmed.Validate(); err != nil {
			return fmt.Errorf("programmed condition: %w", err)
		}
		if g.Programmed.Type != "Programmed" {
			return fmt.Errorf("gateway condition type must be Programmed")
		}
	}
	return nil
}

type Listener struct {
	Name             string            `json:"name" yaml:"name"`
	Hostnames        []string          `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
	Port             int32             `json:"port" yaml:"port"`
	Protocol         string            `json:"protocol" yaml:"protocol"`
	RouteConstraints []string          `json:"routeConstraints,omitempty" yaml:"routeConstraints,omitempty"`
	Conditions       []model.Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

func (l Listener) Validate() error {
	if l.Name == "" {
		return fmt.Errorf("listener name is required")
	}
	if l.Port < 1 || l.Port > 65535 {
		return fmt.Errorf("listener port must be between 1 and 65535")
	}
	if l.Protocol == "" {
		return fmt.Errorf("listener protocol is required")
	}
	for i, condition := range l.Conditions {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i, err)
		}
	}
	return nil
}

type ParentReference struct {
	Identity    model.ResourceIdentity `json:"identity" yaml:"identity"`
	SectionName string                 `json:"sectionName,omitempty" yaml:"sectionName,omitempty"`
}

type RouteAcceptance struct {
	Parent       ParentReference `json:"parent" yaml:"parent"`
	ListenerName string          `json:"listenerName,omitempty" yaml:"listenerName,omitempty"`
	State        RouteState      `json:"state" yaml:"state"`
	Reason       string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message      string          `json:"message,omitempty" yaml:"message,omitempty"`
}

func (a RouteAcceptance) Validate() error {
	if err := a.Parent.Identity.Validate(); err != nil {
		return fmt.Errorf("parent reference: %w", err)
	}
	if err := a.State.Validate(); err != nil {
		return err
	}
	return nil
}

type Route struct {
	Identity   model.ResourceIdentity `json:"identity" yaml:"identity"`
	Hostnames  []string               `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
	ParentRefs []ParentReference      `json:"parentRefs,omitempty" yaml:"parentRefs,omitempty"`
	Acceptance []RouteAcceptance      `json:"acceptance,omitempty" yaml:"acceptance,omitempty"`
	Rules      []Rule                 `json:"rules,omitempty" yaml:"rules,omitempty"`
}

func (r Route) Validate() error {
	if err := r.Identity.Validate(); err != nil {
		return err
	}
	switch r.Identity.Kind {
	case "HTTPRoute", "GRPCRoute":
	default:
		return fmt.Errorf("unsupported route kind %q", r.Identity.Kind)
	}
	for i, parent := range r.ParentRefs {
		if err := parent.Identity.Validate(); err != nil {
			return fmt.Errorf("parent reference %d: %w", i, err)
		}
	}
	for i, acceptance := range r.Acceptance {
		if err := acceptance.Validate(); err != nil {
			return fmt.Errorf("acceptance %d: %w", i, err)
		}
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return nil
}

type Rule struct {
	Index         int             `json:"index" yaml:"index"`
	ListenerNames []string        `json:"listenerNames,omitempty" yaml:"listenerNames,omitempty"`
	Matches       []Match         `json:"matches,omitempty" yaml:"matches,omitempty"`
	Backends      []Backend       `json:"backends,omitempty" yaml:"backends,omitempty"`
	Precedence    PrecedenceState `json:"precedence" yaml:"precedence"`
	CreatedAt     *metav1.Time    `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
}

func (r Rule) Validate() error {
	if r.Index < 1 {
		return fmt.Errorf("rule index must be greater than zero")
	}
	if err := r.Precedence.Validate(); err != nil {
		return err
	}
	for i, backend := range r.Backends {
		if err := backend.Validate(); err != nil {
			return fmt.Errorf("backend %d: %w", i, err)
		}
	}
	return nil
}

type Match struct {
	Path        *PathMatch    `json:"path,omitempty" yaml:"path,omitempty"`
	Headers     []StringMatch `json:"headers,omitempty" yaml:"headers,omitempty"`
	Method      string        `json:"method,omitempty" yaml:"method,omitempty"`
	QueryParams []StringMatch `json:"queryParams,omitempty" yaml:"queryParams,omitempty"`
	GRPC        *GRPCMatch    `json:"grpc,omitempty" yaml:"grpc,omitempty"`
}

type PathMatch struct {
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

type StringMatch struct {
	Name  string `json:"name" yaml:"name"`
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

type GRPCMatch struct {
	Service string `json:"service,omitempty" yaml:"service,omitempty"`
	Method  string `json:"method,omitempty" yaml:"method,omitempty"`
}

type BackendReference struct {
	Group     string `json:"group,omitempty" yaml:"group,omitempty"`
	Kind      string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Name      string `json:"name" yaml:"name"`
	Port      *int32 `json:"port,omitempty" yaml:"port,omitempty"`
}

type Backend struct {
	Reference BackendReference   `json:"reference" yaml:"reference"`
	Grant     *ReferenceGrantUse `json:"referenceGrant,omitempty" yaml:"referenceGrant,omitempty"`
	Service   *BackendService    `json:"service,omitempty" yaml:"service,omitempty"`
}

func (b Backend) Validate() error {
	if b.Reference.Name == "" {
		return fmt.Errorf("backend name is required")
	}
	if b.Reference.Namespace == "" {
		return fmt.Errorf("backend namespace is required")
	}
	if problems := validation.IsDNS1123Subdomain(b.Reference.Name); len(problems) > 0 {
		return fmt.Errorf("invalid backend name %q: %s", b.Reference.Name, problems[0])
	}
	if problems := validation.IsDNS1123Label(b.Reference.Namespace); len(problems) > 0 {
		return fmt.Errorf("invalid backend namespace %q: %s", b.Reference.Namespace, problems[0])
	}
	if b.Reference.Port != nil && (*b.Reference.Port < 1 || *b.Reference.Port > 65535) {
		return fmt.Errorf("backend port must be between 1 and 65535")
	}
	if b.Service != nil {
		if err := b.Service.Validate(); err != nil {
			return fmt.Errorf("service: %w", err)
		}
	}
	return nil
}

type ReferenceGrantUse struct {
	Authorized bool             `json:"authorized" yaml:"authorized"`
	Grants     []ReferenceGrant `json:"grants,omitempty" yaml:"grants,omitempty"`
}

type ReferenceGrant struct {
	Identity model.ResourceIdentity `json:"identity" yaml:"identity"`
	From     []GrantConstraint      `json:"from,omitempty" yaml:"from,omitempty"`
	To       []GrantConstraint      `json:"to,omitempty" yaml:"to,omitempty"`
}

type GrantConstraint struct {
	Group     string `json:"group" yaml:"group"`
	Kind      string `json:"kind" yaml:"kind"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
}

type BackendService struct {
	Identity               model.ResourceIdentity `json:"identity" yaml:"identity"`
	Availability           model.Availability     `json:"availability" yaml:"availability"`
	Ports                  []int32                `json:"ports,omitempty" yaml:"ports,omitempty"`
	PortPresent            *bool                  `json:"portPresent,omitempty" yaml:"portPresent,omitempty"`
	EndpointSlicesSupplied bool                   `json:"endpointSlicesSupplied" yaml:"endpointSlicesSupplied"`
	ReadyAddressCount      int                    `json:"readyAddressCount" yaml:"readyAddressCount"`
}

func (s BackendService) Validate() error {
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	if s.Identity.Kind != "Service" {
		return fmt.Errorf("backend service identity kind must be Service")
	}
	switch s.Availability {
	case model.AvailabilityPresent, model.AvailabilityNotFound, model.AvailabilityUnavailable:
	default:
		return fmt.Errorf("invalid service availability %q", s.Availability)
	}
	if s.ReadyAddressCount < 0 {
		return fmt.Errorf("ready address count cannot be negative")
	}
	for _, port := range s.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("service port must be between 1 and 65535")
		}
	}
	return nil
}

type WaypointSelection struct {
	Resource           model.ResourceIdentity `json:"resource" yaml:"resource"`
	LabelValue         string                 `json:"labelValue" yaml:"labelValue"`
	WaypointNamespace  string                 `json:"waypointNamespace" yaml:"waypointNamespace"`
	WaypointName       string                 `json:"waypointName" yaml:"waypointName"`
	NamespaceQualified bool                   `json:"namespaceQualified" yaml:"namespaceQualified"`
	TargetFound        bool                   `json:"targetFound" yaml:"targetFound"`
	Programmed         *model.Condition       `json:"programmed,omitempty" yaml:"programmed,omitempty"`
}

func (s WaypointSelection) Validate() error {
	if err := s.Resource.Validate(); err != nil {
		return fmt.Errorf("selecting resource: %w", err)
	}
	if s.Resource.Kind != "Namespace" && s.Resource.Kind != "Service" {
		return fmt.Errorf("selector resource kind must be Namespace or Service")
	}
	if s.WaypointNamespace == "" || s.WaypointName == "" {
		return fmt.Errorf("waypoint namespace and name are required")
	}
	if problems := validation.IsDNS1123Label(s.WaypointNamespace); len(problems) > 0 {
		return fmt.Errorf("invalid waypoint namespace %q: %s", s.WaypointNamespace, problems[0])
	}
	if problems := validation.IsDNS1123Subdomain(s.WaypointName); len(problems) > 0 {
		return fmt.Errorf("invalid waypoint name %q: %s", s.WaypointName, problems[0])
	}
	if s.Programmed != nil {
		if err := s.Programmed.Validate(); err != nil {
			return fmt.Errorf("programmed condition: %w", err)
		}
		if s.Programmed.Type != "Programmed" {
			return fmt.Errorf("waypoint condition type must be Programmed")
		}
	}
	return nil
}

type EndpointCheck struct {
	State  EndpointCheckState `json:"state" yaml:"state"`
	Reason string             `json:"reason,omitempty" yaml:"reason,omitempty"`
}

func (c EndpointCheck) Validate() error {
	return c.State.Validate()
}
