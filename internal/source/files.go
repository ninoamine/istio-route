package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ninoamine/istio-route/internal/model"
	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type FilesSource struct {
	Files []string
	Stdin io.Reader
}

func (s *FilesSource) Snapshot(ctx context.Context) (Snapshot, error) {
	if len(s.Files) == 0 {
		return Snapshot{}, &Error{Mode: ModeFiles, Err: fmt.Errorf("at least one manifest file is required")}
	}
	stdinCount := 0
	for _, path := range s.Files {
		if path == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		return Snapshot{}, &Error{Mode: ModeFiles, Err: fmt.Errorf("standard input may be specified only once")}
	}
	scheme, err := manifestScheme()
	if err != nil {
		return Snapshot{}, &Error{Mode: ModeFiles, Err: err}
	}
	snapshot := Snapshot{
		Metadata: model.SourceSnapshot{
			Mode:  model.SourceModeFiles,
			Files: append([]string(nil), s.Files...),
		},
	}
	seen := make(map[string]string)
	for _, path := range s.Files {
		resources, loadErr := readManifestFile(ctx, path, scheme, s.Stdin)
		if loadErr != nil {
			return Snapshot{}, &Error{Mode: ModeFiles, File: path, Err: loadErr}
		}
		for _, resource := range resources {
			key := resourceIdentityKey(resource.Identity)
			if firstPath, exists := seen[key]; exists {
				return Snapshot{}, &Error{
					Mode: ModeFiles, File: path,
					Err: fmt.Errorf(
						"duplicate resource %s %s/%s also found in %q",
						resource.Identity.Kind, resource.Identity.Namespace, resource.Identity.Name, firstPath,
					),
				}
			}
			seen[key] = path
			snapshot.Resources = append(snapshot.Resources, resource)
		}
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, &Error{Mode: ModeFiles, Err: err}
	}
	return snapshot, nil
}

func manifestScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		discoveryv1.AddToScheme,
		istiov1.AddToScheme,
		gatewayv1.Install,
		gatewayv1beta1.Install,
	} {
		if err := addToScheme(scheme); err != nil {
			return nil, fmt.Errorf("register manifest API types: %w", err)
		}
	}
	return scheme, nil
}

func readManifestFile(ctx context.Context, path string, scheme *runtime.Scheme, stdin io.Reader) ([]Resource, error) {
	var (
		contents []byte
		err      error
	)
	if path == "-" {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read manifest: %w", err)
		}
		if stdin == nil {
			stdin = os.Stdin
		}
		contents, err = io.ReadAll(stdin)
	} else {
		contents, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), 4096)
	var resources []Resource
	for documentNumber := 1; ; documentNumber++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read manifest: %w", err)
		}
		var values map[string]any
		err := decoder.Decode(&values)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode document %d: %w", documentNumber, err)
		}
		if len(values) == 0 {
			continue
		}
		resource, err := decodeManifestResource(values, scheme)
		if err != nil {
			return nil, fmt.Errorf("decode document %d: %w", documentNumber, err)
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func decodeManifestResource(values map[string]any, scheme *runtime.Scheme) (Resource, error) {
	object := &unstructured.Unstructured{Object: values}
	gvk := object.GroupVersionKind()
	if gvk.Empty() || gvk.Version == "" || gvk.Kind == "" {
		return Resource{}, fmt.Errorf("apiVersion and kind are required")
	}

	var result runtime.Object = object
	if typed, err := scheme.New(gvk); err == nil {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(values, typed); err != nil {
			return Resource{}, fmt.Errorf("decode %s: %w", gvk.String(), err)
		}
		typed.GetObjectKind().SetGroupVersionKind(gvk)
		result = typed
	}
	accessor, err := meta.Accessor(result)
	if err != nil {
		return Resource{}, fmt.Errorf("read %s metadata: %w", gvk.String(), err)
	}
	if accessor.GetName() == "" {
		return Resource{}, fmt.Errorf("%s metadata.name is required", gvk.String())
	}

	return Resource{
		Identity: model.ResourceIdentity{
			Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind,
			Namespace: accessor.GetNamespace(), Name: accessor.GetName(),
		},
		GVK:    gvk,
		Object: result,
	}, nil
}

func resourceIdentityKey(identity model.ResourceIdentity) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s",
		identity.Group, identity.Kind, identity.Namespace, identity.Name,
	)
}
