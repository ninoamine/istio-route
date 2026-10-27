package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ErrUnsupportedFamily = errors.New("resource family is not available")

// SnapshotSource loads resources from one configured input mode.
type SnapshotSource interface {
	Snapshot(context.Context) (Snapshot, error)
}

// Snapshot is the shared input to resolution for cluster and manifest sources.
type Snapshot struct {
	Metadata  model.SourceSnapshot `json:"metadata" yaml:"metadata"`
	Resources []Resource           `json:"resources" yaml:"resources"`
}

// Resource preserves both the original API identity and the typed Kubernetes object.
type Resource struct {
	Identity model.ResourceIdentity  `json:"identity" yaml:"identity"`
	GVK      schema.GroupVersionKind `json:"gvk" yaml:"gvk"`
	Object   runtime.Object          `json:"-" yaml:"-"`
}

func (s Snapshot) Validate() error {
	if err := s.Metadata.Validate(); err != nil {
		return fmt.Errorf("snapshot metadata: %w", err)
	}
	for i, resource := range s.Resources {
		if err := resource.Identity.Validate(); err != nil {
			return fmt.Errorf("snapshot resource %d: %w", i, err)
		}
		if resource.GVK.Empty() {
			return fmt.Errorf("snapshot resource %d: group/version/kind is required", i)
		}
		if resource.GVK.Group != resource.Identity.Group ||
			resource.GVK.Version != resource.Identity.Version ||
			resource.GVK.Kind != resource.Identity.Kind {
			return fmt.Errorf("snapshot resource %d: GVK does not match resource identity", i)
		}
		if resource.Object == nil {
			return fmt.Errorf("snapshot resource %d: object is required", i)
		}
	}
	return nil
}

// Error adds source context while preserving the original cause.
type Error struct {
	Mode SourceMode
	File string
	Err  error
}

type SourceMode = model.SourceMode

const (
	ModeCluster = model.SourceModeCluster
	ModeFiles   = model.SourceModeFiles
)

func (e *Error) Error() string {
	if e.File != "" {
		return fmt.Sprintf("load %s source file %q: %v", e.Mode, e.File, e.Err)
	}
	return fmt.Sprintf("load %s source: %v", e.Mode, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}
