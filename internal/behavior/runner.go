package behavior

import (
	"bytes"
	"context"
	"encoding/xml"
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
	ManifestName string
	Trace        TraceFunc
	Output       io.Writer
	Diagnostics  io.Writer
	JUnitPath    string
}

func writeConsoleReport(output io.Writer, scenarios []scenarioResult) error {
	passed := 0
	failed := 0
	previousFeature := ""
	for _, scenario := range scenarios {
		if scenario.feature != previousFeature {
			if _, err := fmt.Fprintf(output, "Feature: %s (%s)\n", scenario.feature, scenario.uri); err != nil {
				return err
			}
			previousFeature = scenario.feature
		}
		if scenario.err == nil {
			passed++
			if _, err := fmt.Fprintf(output, "  PASS: %s\n", scenario.name); err != nil {
				return err
			}
			continue
		}
		failed++
		if _, err := fmt.Fprintf(output, "  FAIL: %s (%s:%d)\n    %s\n", scenario.name, scenario.uri, scenario.line, scenario.err); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(output, "%d scenarios: %d passed, %d failed\n", len(scenarios), passed, failed)
	return err
}

type RunResult struct {
	Failed  bool
	Invalid bool
}

type scenarioResult struct {
	feature string
	name    string
	uri     string
	line    int64
	err     error
}

type resultCollector struct {
	mu        sync.Mutex
	scenarios []scenarioResult
	invalid   bool
}

func Run(ctx context.Context, options RunOptions) (RunResult, error) {
	if options.Trace == nil {
		return RunResult{}, fmt.Errorf("run behavior tests: trace function is required")
	}
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

	var console bytes.Buffer
	collector := &resultCollector{}
	featureNames := make(map[string]string)
	godogOptions := &godog.Options{
		Paths:          paths,
		Format:         "progress",
		Output:         &console,
		NoColors:       true,
		Strict:         true,
		Concurrency:    1,
		DefaultContext: ctx,
		FS:             diskFeatureFS{},
	}
	suite := godog.TestSuite{
		Name:    "istio-route behavior",
		Options: godogOptions,
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			state := &scenarioState{
				trace:        options.Trace,
				manifestName: options.ManifestName,
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
					feature: featureNames[scenario.Uri],
					name:    scenario.Name,
					uri:     scenario.Uri,
					line:    line,
					err:     scenarioErr,
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
		return RunResult{Invalid: true}, fmt.Errorf("read feature files: %w", err)
	}
	runnable := 0
	for _, feature := range features {
		runnable += len(feature.Pickles)
		featureNames[feature.Uri] = feature.Feature.Name
	}
	if runnable == 0 {
		return RunResult{Invalid: true}, fmt.Errorf("read feature files: no runnable scenarios found")
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
	if err := writeConsoleReport(options.Output, scenarios); err != nil {
		return RunResult{}, fmt.Errorf("write behavior test output: %w", err)
	}
	for _, scenario := range scenarios {
		if scenario.err == nil || !isInvalidStepError(scenario.err) {
			continue
		}
		step := undefinedStepText(scenario.err.Error())
		suggestion := suggestStep(step)
		if suggestion != "" {
			if _, err := fmt.Fprintf(options.Diagnostics, "%s: unsupported or ambiguous step %q; closest supported step: %q\n", scenario.uri, step, suggestion); err != nil {
				return result, fmt.Errorf("write step suggestion: %w", err)
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
		errors.Is(err, godog.ErrPending)
}

func undefinedStepText(message string) string {
	for _, marker := range []string{
		godog.ErrUndefined.Error() + ": ",
		godog.ErrAmbiguous.Error() + ", step text: ",
	} {
		if _, after, ok := strings.Cut(message, marker); ok {
			return strings.TrimSpace(after)
		}
	}
	return message
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

type junitReport struct {
	XMLName   xml.Name    `xml:"testsuite"`
	Name      string      `xml:"name,attr"`
	Tests     int         `xml:"tests,attr"`
	Failures  int         `xml:"failures,attr"`
	TestCases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr,omitempty"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func writeJUnit(ctx context.Context, path string, scenarios []scenarioResult) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write JUnit report: %w", err)
	}
	report := junitReport{Name: "istio-route behavior", Tests: len(scenarios)}
	for _, scenario := range scenarios {
		testCase := junitCase{Name: scenario.name, ClassName: scenario.uri}
		if scenario.err != nil {
			report.Failures++
			testCase.Failure = &junitFailure{
				Message: scenario.err.Error(),
				Text:    scenario.err.Error(),
			}
		}
		report.TestCases = append(report.TestCases, testCase)
	}
	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JUnit XML: %w", err)
	}
	data = append([]byte(xml.Header), data...)
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}
	return nil
}
