package behavior

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnknownEnvironment     = errors.New("unknown manifest environment")
	ErrConflictingEnvironment = errors.New("conflicting manifest environment")
)

type ManifestSet struct {
	Name  string
	Files []string
}

type Environment struct {
	Name  string
	Trace TraceFunc
}

func ParseManifestSets(files, namedFiles []string) ([]ManifestSet, error) {
	if len(files) > 0 && len(namedFiles) > 0 {
		return nil, fmt.Errorf("cannot mix unnamed manifest files with named manifest sets")
	}
	if len(files) == 0 && len(namedFiles) == 0 {
		return nil, fmt.Errorf("at least one rendered manifest file is required with -f or --manifests")
	}

	sets := make([]ManifestSet, 0, len(namedFiles))
	if len(files) > 0 {
		if err := validateManifestPaths(files); err != nil {
			return nil, err
		}
		return []ManifestSet{{Files: append([]string(nil), files...)}}, nil
	}

	setIndexes := make(map[string]int)
	seenFiles := make(map[string]map[string]struct{})
	for _, value := range namedFiles {
		name, path, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("manifest set %q must use name=file syntax", value)
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("manifest set %q: name cannot be empty", value)
		}
		if strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" {
			return nil, fmt.Errorf("manifest set %q: name may contain only letters, numbers, '.', '_' and '-'", name)
		}
		if path == "" {
			return nil, fmt.Errorf("manifest set %q: file path cannot be empty", name)
		}

		index, exists := setIndexes[name]
		if !exists {
			index = len(sets)
			setIndexes[name] = index
			sets = append(sets, ManifestSet{Name: name})
			seenFiles[name] = make(map[string]struct{})
		}
		if _, duplicate := seenFiles[name][path]; duplicate {
			return nil, fmt.Errorf("manifest set %q: duplicate manifest file %q", name, path)
		}
		seenFiles[name][path] = struct{}{}
		sets[index].Files = append(sets[index].Files, path)
	}

	var allPaths []string
	for _, set := range sets {
		allPaths = append(allPaths, set.Files...)
	}
	if err := validateManifestPaths(allPaths); err != nil {
		return nil, err
	}
	return sets, nil
}

func validateManifestPaths(paths []string) error {
	stdinCount := 0
	for _, path := range paths {
		if path == "" {
			return fmt.Errorf("manifest file path cannot be empty")
		}
		if path == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		return fmt.Errorf("standard input may be specified only once")
	}
	return nil
}
