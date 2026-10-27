package model

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation"
)

type Origin string

const (
	OriginIstio      Origin = "Istio"
	OriginGatewayAPI Origin = "Gateway API"
)

type SourceMode string

const (
	SourceModeCluster SourceMode = "cluster"
	SourceModeFiles   SourceMode = "files"
)

type ConditionStatus string

const (
	ConditionStatusTrue        ConditionStatus = "True"
	ConditionStatusFalse       ConditionStatus = "False"
	ConditionStatusUnknown     ConditionStatus = "Unknown"
	ConditionStatusUnavailable ConditionStatus = "Unavailable"
)

type FindingSeverity string

const (
	FindingSeverityInfo    FindingSeverity = "info"
	FindingSeverityWarning FindingSeverity = "warning"
	FindingSeverityError   FindingSeverity = "error"
)

type DestinationKind string

const (
	DestinationKindService  DestinationKind = "service"
	DestinationKindExternal DestinationKind = "external"
)

type ResourceIdentity struct {
	Group     string `json:"group,omitempty" yaml:"group,omitempty"`
	Version   string `json:"version" yaml:"version"`
	Kind      string `json:"kind" yaml:"kind"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Name      string `json:"name" yaml:"name"`
}

func (r ResourceIdentity) Validate() error {
	if r.Version == "" {
		return fmt.Errorf("resource version is required")
	}
	if r.Kind == "" {
		return fmt.Errorf("resource kind is required")
	}
	if r.Name == "" {
		return fmt.Errorf("resource name is required")
	}
	if problems := validation.IsDNS1123Subdomain(r.Name); len(problems) > 0 {
		return fmt.Errorf("invalid resource name %q: %s", r.Name, problems[0])
	}
	if r.Namespace != "" {
		if problems := validation.IsDNS1123Label(r.Namespace); len(problems) > 0 {
			return fmt.Errorf("invalid resource namespace %q: %s", r.Namespace, problems[0])
		}
	}
	if r.Group != "" {
		if problems := validation.IsDNS1123Subdomain(r.Group); len(problems) > 0 {
			return fmt.Errorf("invalid resource group %q: %s", r.Group, problems[0])
		}
	}
	return nil
}

type Analysis struct {
	Hostname    string         `json:"hostname" yaml:"hostname"`
	Source      SourceSnapshot `json:"source" yaml:"source"`
	EntryPoints []EntryPoint   `json:"entryPoints,omitempty" yaml:"entryPoints,omitempty"`
	Routes      []Route        `json:"routes,omitempty" yaml:"routes,omitempty"`
	Findings    []Finding      `json:"findings,omitempty" yaml:"findings,omitempty"`
}

func (a Analysis) Validate() error {
	if a.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if err := a.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	for i, entry := range a.EntryPoints {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("entry point %d: %w", i, err)
		}
	}
	for i, route := range a.Routes {
		if err := route.Validate(); err != nil {
			return fmt.Errorf("route %d: %w", i, err)
		}
	}
	for i, finding := range a.Findings {
		if err := finding.Validate(); err != nil {
			return fmt.Errorf("finding %d: %w", i, err)
		}
	}
	return nil
}

type SourceSnapshot struct {
	Mode      SourceMode             `json:"mode" yaml:"mode"`
	Context   string                 `json:"context,omitempty" yaml:"context,omitempty"`
	Namespace string                 `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Files     []string               `json:"files,omitempty" yaml:"files,omitempty"`
	Families  []ResourceFamilyStatus `json:"families,omitempty" yaml:"families,omitempty"`
}

func (s SourceSnapshot) Validate() error {
	switch s.Mode {
	case SourceModeCluster:
		if len(s.Files) != 0 {
			return fmt.Errorf("cluster source cannot include files")
		}
	case SourceModeFiles:
		if len(s.Files) == 0 {
			return fmt.Errorf("files source requires at least one file")
		}
		for i, file := range s.Files {
			if file == "" {
				return fmt.Errorf("file path %d is empty", i)
			}
		}
	default:
		return fmt.Errorf("invalid source mode %q", s.Mode)
	}
	if s.Namespace != "" {
		if problems := validation.IsDNS1123Label(s.Namespace); len(problems) > 0 {
			return fmt.Errorf("invalid source namespace %q: %s", s.Namespace, problems[0])
		}
	}
	return nil
}

type ResourceFamilyStatus struct {
	Family    string `json:"family" yaml:"family"`
	Available bool   `json:"available" yaml:"available"`
	Reason    string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type EntryPoint struct {
	Origin           Origin           `json:"origin" yaml:"origin"`
	Resource         ResourceIdentity `json:"resource" yaml:"resource"`
	Name             string           `json:"name" yaml:"name"`
	Hostname         string           `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Port             int32            `json:"port,omitempty" yaml:"port,omitempty"`
	Protocol         string           `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	TLS              *TLSConfig       `json:"tls,omitempty" yaml:"tls,omitempty"`
	RouteConstraints []string         `json:"routeConstraints,omitempty" yaml:"routeConstraints,omitempty"`
	GatewayClass     *GatewayClassRef `json:"gatewayClass,omitempty" yaml:"gatewayClass,omitempty"`
	Conditions       []Condition      `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

func (e EntryPoint) Validate() error {
	if !validOrigin(e.Origin) {
		return fmt.Errorf("invalid origin %q", e.Origin)
	}
	if err := e.Resource.Validate(); err != nil {
		return fmt.Errorf("resource: %w", err)
	}
	if e.Name == "" {
		return fmt.Errorf("entry point name is required")
	}
	if e.TLS != nil {
		for i, certificate := range e.TLS.CertificateRefs {
			if err := certificate.Validate(); err != nil {
				return fmt.Errorf("TLS certificate reference %d: %w", i, err)
			}
		}
	}
	if e.GatewayClass != nil {
		if err := e.GatewayClass.Identity.Validate(); err != nil {
			return fmt.Errorf("GatewayClass identity: %w", err)
		}
		if e.GatewayClass.Controller == "" {
			return fmt.Errorf("GatewayClass controller is required")
		}
	}
	for i, condition := range e.Conditions {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i, err)
		}
	}
	return nil
}

type TLSConfig struct {
	Mode            string             `json:"mode,omitempty" yaml:"mode,omitempty"`
	CertificateRefs []ResourceIdentity `json:"certificateRefs,omitempty" yaml:"certificateRefs,omitempty"`
}

type GatewayClassRef struct {
	Identity   ResourceIdentity `json:"identity" yaml:"identity"`
	Controller string           `json:"controller" yaml:"controller"`
}

type Route struct {
	Origin     Origin             `json:"origin" yaml:"origin"`
	Resource   ResourceIdentity   `json:"resource" yaml:"resource"`
	Hostnames  []string           `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
	ParentRefs []ResourceIdentity `json:"parentRefs,omitempty" yaml:"parentRefs,omitempty"`
	Conditions []Condition        `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Rules      []Rule             `json:"rules,omitempty" yaml:"rules,omitempty"`
}

func (r Route) Validate() error {
	if !validOrigin(r.Origin) {
		return fmt.Errorf("invalid origin %q", r.Origin)
	}
	if err := r.Resource.Validate(); err != nil {
		return fmt.Errorf("resource: %w", err)
	}
	for i, parent := range r.ParentRefs {
		if err := parent.Validate(); err != nil {
			return fmt.Errorf("parent reference %d: %w", i, err)
		}
	}
	for i, condition := range r.Conditions {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i, err)
		}
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return nil
}

type Condition struct {
	Type               string          `json:"type" yaml:"type"`
	Status             ConditionStatus `json:"status" yaml:"status"`
	Reason             string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string          `json:"message,omitempty" yaml:"message,omitempty"`
	ObservedGeneration *int64          `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
}

func (c Condition) Validate() error {
	if c.Type == "" {
		return fmt.Errorf("condition type is required")
	}
	switch c.Status {
	case ConditionStatusTrue, ConditionStatusFalse, ConditionStatusUnknown, ConditionStatusUnavailable:
		return nil
	default:
		return fmt.Errorf("invalid condition status %q", c.Status)
	}
}

type Rule struct {
	Index        int               `json:"index" yaml:"index"`
	Matches      []Match           `json:"matches,omitempty" yaml:"matches,omitempty"`
	Filters      []Filter          `json:"filters,omitempty" yaml:"filters,omitempty"`
	Actions      map[string]string `json:"actions,omitempty" yaml:"actions,omitempty"`
	Destinations []Destination     `json:"destinations,omitempty" yaml:"destinations,omitempty"`
}

func (r Rule) Validate() error {
	if r.Index < 1 {
		return fmt.Errorf("rule index must be greater than zero")
	}
	for i, destination := range r.Destinations {
		if err := destination.Validate(); err != nil {
			return fmt.Errorf("destination %d: %w", i, err)
		}
	}
	return nil
}

type Match struct {
	PathType    string            `json:"pathType,omitempty" yaml:"pathType,omitempty"`
	Path        string            `json:"path,omitempty" yaml:"path,omitempty"`
	Headers     map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	QueryParams map[string]string `json:"queryParams,omitempty" yaml:"queryParams,omitempty"`
	Method      string            `json:"method,omitempty" yaml:"method,omitempty"`
}

type Filter struct {
	Type    string            `json:"type" yaml:"type"`
	Details map[string]string `json:"details,omitempty" yaml:"details,omitempty"`
}

type Destination struct {
	Kind            DestinationKind      `json:"kind" yaml:"kind"`
	BackendGroup    string               `json:"backendGroup,omitempty" yaml:"backendGroup,omitempty"`
	BackendKind     string               `json:"backendKind,omitempty" yaml:"backendKind,omitempty"`
	Namespace       string               `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Name            string               `json:"name" yaml:"name"`
	Subset          string               `json:"subset,omitempty" yaml:"subset,omitempty"`
	DestinationRule *ResourceIdentity    `json:"destinationRule,omitempty" yaml:"destinationRule,omitempty"`
	SubsetLabels    map[string]string    `json:"subsetLabels,omitempty" yaml:"subsetLabels,omitempty"`
	Port            *int32               `json:"port,omitempty" yaml:"port,omitempty"`
	Weight          *int32               `json:"weight,omitempty" yaml:"weight,omitempty"`
	ReferenceGrant  *GrantResult         `json:"referenceGrant,omitempty" yaml:"referenceGrant,omitempty"`
	External        *ExternalDestination `json:"external,omitempty" yaml:"external,omitempty"`
	Service         *Service             `json:"service,omitempty" yaml:"service,omitempty"`
}

func (d Destination) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("destination name is required")
	}
	if d.Weight != nil && *d.Weight < 0 {
		return fmt.Errorf("negative weight %d", *d.Weight)
	}
	if d.DestinationRule != nil {
		if err := d.DestinationRule.Validate(); err != nil {
			return fmt.Errorf("DestinationRule: %w", err)
		}
	}
	switch d.Kind {
	case DestinationKindService:
		if d.External != nil {
			return fmt.Errorf("service destination cannot contain external details")
		}
		if d.Namespace != "" {
			if problems := validation.IsDNS1123Label(d.Namespace); len(problems) > 0 {
				return fmt.Errorf("invalid destination namespace %q: %s", d.Namespace, problems[0])
			}
		}
		if problems := validation.IsDNS1123Subdomain(d.Name); len(problems) > 0 {
			return fmt.Errorf("invalid destination name %q: %s", d.Name, problems[0])
		}
	case DestinationKindExternal:
		if d.External == nil {
			return fmt.Errorf("external destination requires external details")
		}
	case "":
		return fmt.Errorf("destination kind is required")
	default:
		return fmt.Errorf("invalid destination kind %q", d.Kind)
	}
	if d.Service != nil {
		if err := d.Service.Validate(); err != nil {
			return fmt.Errorf("service: %w", err)
		}
	}
	return nil
}

type GrantResult struct {
	Allowed  bool              `json:"allowed" yaml:"allowed"`
	GrantRef *ResourceIdentity `json:"grantRef,omitempty" yaml:"grantRef,omitempty"`
	Reason   string            `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type ExternalDestination struct {
	Hosts      []string       `json:"hosts,omitempty" yaml:"hosts,omitempty"`
	Ports      []ExternalPort `json:"ports,omitempty" yaml:"ports,omitempty"`
	Location   string         `json:"location,omitempty" yaml:"location,omitempty"`
	Resolution string         `json:"resolution,omitempty" yaml:"resolution,omitempty"`
}

type ExternalPort struct {
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Number   uint32 `json:"number" yaml:"number"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
}

type Service struct {
	Identity     ResourceIdentity `json:"identity" yaml:"identity"`
	Ports        []ServicePort    `json:"ports,omitempty" yaml:"ports,omitempty"`
	Availability Availability     `json:"availability" yaml:"availability"`
	Endpoints    []Endpoint       `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
}

func (s Service) Validate() error {
	if err := s.Identity.Validate(); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	if s.Identity.Kind != "Service" {
		return fmt.Errorf("service identity kind must be Service")
	}
	if !validAvailability(s.Availability) {
		return fmt.Errorf("invalid service availability %q", s.Availability)
	}
	for i, endpoint := range s.Endpoints {
		if err := endpoint.Validate(); err != nil {
			return fmt.Errorf("endpoint %d: %w", i, err)
		}
	}
	return nil
}

type ServicePort struct {
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Port     int32  `json:"port" yaml:"port"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
}

type Endpoint struct {
	Addresses   []string          `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Ports       []ServicePort     `json:"ports,omitempty" yaml:"ports,omitempty"`
	Ready       *bool             `json:"ready,omitempty" yaml:"ready,omitempty"`
	Serving     *bool             `json:"serving,omitempty" yaml:"serving,omitempty"`
	Terminating *bool             `json:"terminating,omitempty" yaml:"terminating,omitempty"`
	Target      *ResourceIdentity `json:"target,omitempty" yaml:"target,omitempty"`
	Pod         *Pod              `json:"pod,omitempty" yaml:"pod,omitempty"`
}

func (e Endpoint) Validate() error {
	if e.Target != nil {
		if err := e.Target.Validate(); err != nil {
			return fmt.Errorf("target: %w", err)
		}
	}
	if e.Pod != nil {
		if err := e.Pod.Validate(); err != nil {
			return fmt.Errorf("pod: %w", err)
		}
	}
	return nil
}

type Pod struct {
	Identity     ResourceIdentity `json:"identity" yaml:"identity"`
	Ready        *bool            `json:"ready,omitempty" yaml:"ready,omitempty"`
	Phase        string           `json:"phase,omitempty" yaml:"phase,omitempty"`
	Availability Availability     `json:"availability" yaml:"availability"`
}

func (p Pod) Validate() error {
	if err := p.Identity.Validate(); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	if p.Identity.Kind != "Pod" {
		return fmt.Errorf("pod identity kind must be Pod")
	}
	if !validAvailability(p.Availability) {
		return fmt.Errorf("invalid pod availability %q", p.Availability)
	}
	return nil
}

type Availability string

const (
	AvailabilityPresent     Availability = "present"
	AvailabilityNotFound    Availability = "not-found"
	AvailabilityUnavailable Availability = "unavailable"
)

type Finding struct {
	Code      string             `json:"code" yaml:"code"`
	Severity  FindingSeverity    `json:"severity" yaml:"severity"`
	Message   string             `json:"message" yaml:"message"`
	Resources []ResourceIdentity `json:"resources,omitempty" yaml:"resources,omitempty"`
}

func (f Finding) Validate() error {
	if f.Code == "" {
		return fmt.Errorf("finding code is required")
	}
	if f.Message == "" {
		return fmt.Errorf("finding message is required")
	}
	switch f.Severity {
	case FindingSeverityInfo, FindingSeverityWarning, FindingSeverityError:
	default:
		return fmt.Errorf("invalid finding severity %q", f.Severity)
	}
	for i, resource := range f.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("resource %d: %w", i, err)
		}
	}
	return nil
}

func validOrigin(origin Origin) bool {
	return origin == OriginIstio || origin == OriginGatewayAPI
}

func validAvailability(availability Availability) bool {
	switch availability {
	case AvailabilityPresent, AvailabilityNotFound, AvailabilityUnavailable:
		return true
	default:
		return false
	}
}
