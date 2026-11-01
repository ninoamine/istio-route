package behavior

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/cucumber/godog"
)

type RunOptions struct {
	Paths        []string
	Environments []Environment
	Output       io.Writer
	Diagnostics  io.Writer
	JUnitPath    string
}

type RunResult struct {
	Failed  bool
	Invalid bool
}

type scenarioResult struct {
	feature     string
	name        string
	environment string
	pickleID    string
	uri         string
	line        int64
	err         error
	steps       []stepEvent
}

type resultCollector struct {
	mu        sync.Mutex
	scenarios []scenarioResult
	invalid   bool
}

func Run(ctx context.Context, options RunOptions) (RunResult, error) {
	if options.Output == nil {
		return RunResult{}, fmt.Errorf("run behavior tests: output writer is required")
	}
	if options.Diagnostics == nil {
		return RunResult{}, fmt.Errorf("run behavior tests: diagnostics writer is required")
	}
	paths, err := discoverFeatureFiles(options.Paths)
	if err != nil {
		return RunResult{Invalid: true}, err
	}
	if err := ctx.Err(); err != nil {
		return RunResult{Invalid: true}, fmt.Errorf("run behavior tests: %w", err)
	}

	if len(options.Environments) == 0 {
		return RunResult{Invalid: true}, fmt.Errorf("run behavior tests: at least one manifest environment is required")
	}
	environmentNames := make([]string, 0, len(options.Environments))
	seenEnvironments := make(map[string]struct{}, len(options.Environments))
	for _, environment := range options.Environments {
		if environment.Trace == nil {
			return RunResult{Invalid: true}, fmt.Errorf("run behavior tests: trace function is required for environment %q", environment.Name)
		}
		if _, exists := seenEnvironments[environment.Name]; exists {
			return RunResult{Invalid: true}, fmt.Errorf("run behavior tests: duplicate manifest environment %q", environment.Name)
		}
		seenEnvironments[environment.Name] = struct{}{}
		environmentNames = append(environmentNames, environment.Name)
	}
	if len(options.Environments) > 1 {
		for _, environment := range options.Environments {
			if environment.Name == "" {
				return RunResult{Invalid: true}, fmt.Errorf("run behavior tests: cannot mix unnamed and named manifest environments")
			}
		}
	}

	var (
		result    RunResult
		scenarios []scenarioResult
	)
	for _, environment := range options.Environments {
		runResult, runScenarios, err := runEnvironment(ctx, paths, environment, environmentNames)
		if err != nil {
			return result, err
		}
		result.Failed = result.Failed || runResult.Failed
		result.Invalid = result.Invalid || runResult.Invalid
		scenarios = append(scenarios, runScenarios...)
	}
	if len(scenarios) == 0 {
		return RunResult{Invalid: true}, fmt.Errorf("read feature files: no scenarios match the supplied manifest environments")
	}
	if err := writeConsoleReport(options.Output, scenarios); err != nil {
		return RunResult{}, fmt.Errorf("write behavior test output: %w", err)
	}
	for _, scenario := range scenarios {
		for _, stepEvent := range scenario.steps {
			if stepEvent.status != "undefined" && stepEvent.status != "ambiguous" {
				continue
			}
			suggestion := suggestStep(stepEvent.text)
			if suggestion != "" {
				if _, err := fmt.Fprintf(
					options.Diagnostics,
					"%s:%d: unsupported or ambiguous step %q; closest supported step: %q\n",
					scenario.uri, stepEvent.line, stepEvent.text, suggestion,
				); err != nil {
					return result, fmt.Errorf("write step suggestion: %w", err)
				}
			}
		}
	}

	if options.JUnitPath != "" {
		if err := writeJUnit(ctx, options.JUnitPath, scenarios); err != nil {
			return result, fmt.Errorf("write JUnit report %q: %w", options.JUnitPath, err)
		}
	}
	return result, nil
}

func runEnvironment(
	ctx context.Context,
	paths []string,
	environment Environment,
	environmentNames []string,
) (RunResult, []scenarioResult, error) {
	collector := &resultCollector{}
	suiteName, formatter := newFormatterCollector()
	defer reportFormatterRuns.Delete(suiteName)
	featureNames := make(map[string]string)
	godogOptions := &godog.Options{
		Paths:          paths,
		Tags:           environmentTagExpression(environment.Name, environmentNames),
		Format:         reportFormatterName,
		Output:         io.Discard,
		NoColors:       true,
		Strict:         true,
		Concurrency:    1,
		DefaultContext: ctx,
		FS:             diskFeatureFS{},
	}
	suite := godog.TestSuite{
		Name:    suiteName,
		Options: godogOptions,
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			state := &scenarioState{
				trace:            environment.Trace,
				manifestName:     environment.Name,
				environmentNames: environmentNames,
			}
			scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				state.reset()
				return ctx, nil
			})
			scenarioContext.After(func(ctx context.Context, scenario *godog.Scenario, scenarioErr error) (context.Context, error) {
				var line int64
				if scenario.Location != nil {
					line = scenario.Location.Line
				}
				collector.add(scenarioResult{
					feature:     featureNames[scenario.Uri],
					name:        scenario.Name,
					environment: environment.Name,
					pickleID:    scenario.Id,
					uri:         scenario.Uri,
					line:        line,
					err:         scenarioErr,
				})
				if isInvalidStepError(scenarioErr) {
					collector.setInvalid()
				}
				return ctx, nil
			})
			registerSteps(scenarioContext, state)
		},
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		return RunResult{Invalid: true}, nil, fmt.Errorf("read feature files: %w", err)
	}
	runnable := 0
	for _, feature := range features {
		runnable += len(feature.Pickles)
		featureNames[feature.Uri] = feature.Feature.Name
	}
	if runnable == 0 {
		return RunResult{}, nil, nil
	}

	exitCode := suite.Run()
	result := collector.result()
	if exitCode != 0 {
		result.Failed = true
	}
	if exitCode == 2 {
		result.Invalid = true
	}
	scenarios := collector.snapshot()
	for index := range scenarios {
		if feature := formatter.feature(scenarios[index].uri); feature.name != "" {
			scenarios[index].feature = feature.name
		}
		if pickle := formatter.scenario(scenarios[index].pickleID); pickle.name != "" {
			scenarios[index].name = pickle.name
			if pickle.line != 0 {
				scenarios[index].line = pickle.line
			}
		}
		scenarios[index].steps = formatter.steps(scenarios[index].pickleID)
	}
	return result, scenarios, nil
}

func (c *resultCollector) add(result scenarioResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scenarios = append(c.scenarios, result)
}

func (c *resultCollector) setInvalid() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalid = true
}

func (c *resultCollector) result() RunResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := RunResult{Invalid: c.invalid}
	for _, scenario := range c.scenarios {
		if scenario.err != nil {
			result.Failed = true
			if isInvalidStepError(scenario.err) {
				result.Invalid = true
			}
		}
	}
	return result
}

func (c *resultCollector) snapshot() []scenarioResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]scenarioResult(nil), c.scenarios...)
}

func isInvalidStepError(err error) bool {
	return errors.Is(err, godog.ErrUndefined) ||
		errors.Is(err, godog.ErrAmbiguous) ||
		errors.Is(err, godog.ErrPending) ||
		errors.Is(err, ErrUnknownEnvironment) ||
		errors.Is(err, ErrConflictingEnvironment)
}

func environmentTagExpression(current string, names []string) string {
	if current == "" || len(names) == 0 {
		return ""
	}
	unmatched := make([]string, 0, len(names))
	for _, name := range names {
		unmatched = append(unmatched, "~@"+name)
	}
	return strings.Join([]string{strings.Join(unmatched, "&&"), "@" + current}, ",")
}

func discoverFeatureFiles(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("at least one feature file or directory is required")
	}
	files := make([]string, 0)
	seen := make(map[string]struct{})
	for _, input := range inputs {
		absolute, err := filepath.Abs(input)
		if err != nil {
			return nil, fmt.Errorf("resolve feature path %q: %w", input, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("read feature path %q: %w", input, err)
		}
		foundBefore := len(files)
		if info.IsDir() {
			err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return fmt.Errorf("walk feature directory %q: %w", input, walkErr)
				}
				if entry.IsDir() || filepath.Ext(path) != ".feature" {
					return nil
				}
				return appendFeatureFile(&files, seen, path)
			})
			if err != nil {
				return nil, err
			}
			if len(files) == foundBefore {
				return nil, fmt.Errorf("feature directory %q contains no .feature files", input)
			}
			continue
		}
		if filepath.Ext(absolute) != ".feature" {
			return nil, fmt.Errorf("feature file %q must have the .feature extension", input)
		}
		if err := appendFeatureFile(&files, seen, absolute); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no feature files found")
	}
	sort.Strings(files)
	return files, nil
}

type diskFeatureFS struct{}

func (diskFeatureFS) Open(name string) (fs.File, error) {
	return os.Open(name)
}

func appendFeatureFile(files *[]string, seen map[string]struct{}, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve feature file %q: %w", path, err)
	}
	if _, exists := seen[absolute]; exists {
		return nil
	}
	file, err := os.Open(absolute)
	if err != nil {
		return fmt.Errorf("read feature file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close feature file %q: %w", path, err)
	}
	seen[absolute] = struct{}{}
	*files = append(*files, absolute)
	return nil
}
