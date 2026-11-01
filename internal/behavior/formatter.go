package behavior

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/formatters"
	messages "github.com/cucumber/messages/go/v34"
)

const reportFormatterName = "istio-route-report"

var (
	reportFormatterRuns sync.Map
	reportRunSequence   atomic.Uint64
)

type stepEvent struct {
	text   string
	status string
	line   int64
	err    error
}

type formatterCollector struct {
	mu         sync.Mutex
	features   map[string]featureEvents
	pickleIDs  map[string]scenarioEvents
	stepEvents map[string][]stepEvent
}

type featureEvents struct {
	name         string
	stepLineByID map[string]int64
}

type scenarioEvents struct {
	uri  string
	name string
	line int64
}

func init() {
	godog.Format(reportFormatterName, "Collects scenario and step results for istio-route reports.", func(name string, _ io.Writer) godog.Formatter {
		value, ok := reportFormatterRuns.Load(name)
		collector, valid := value.(*formatterCollector)
		if !ok || !valid {
			return &reportFormatter{}
		}
		return &reportFormatter{collector: collector}
	})
}

func newFormatterCollector() (string, *formatterCollector) {
	name := fmt.Sprintf("istio-route behavior %d", reportRunSequence.Add(1))
	collector := &formatterCollector{
		features:   make(map[string]featureEvents),
		pickleIDs:  make(map[string]scenarioEvents),
		stepEvents: make(map[string][]stepEvent),
	}
	reportFormatterRuns.Store(name, collector)
	return name, collector
}

type reportFormatter struct {
	collector *formatterCollector
}

func (f *reportFormatter) TestRunStarted() {}

func (f *reportFormatter) Feature(document *messages.GherkinDocument, uri string, _ []byte) {
	if f.collector == nil || document == nil || document.Feature == nil {
		return
	}
	feature := featureEvents{
		name:         document.Feature.Name,
		stepLineByID: make(map[string]int64),
	}
	collectFeatureStepLines(document.Feature, feature.stepLineByID)
	f.collector.mu.Lock()
	f.collector.features[uri] = feature
	f.collector.mu.Unlock()
}

func (f *reportFormatter) Pickle(pickle *messages.Pickle) {
	if f.collector == nil || pickle == nil {
		return
	}
	event := scenarioEvents{uri: pickle.Uri, name: pickle.Name}
	if pickle.Location != nil {
		event.line = pickle.Location.Line
	}
	f.collector.mu.Lock()
	f.collector.pickleIDs[pickle.Id] = event
	f.collector.mu.Unlock()
}

func (f *reportFormatter) Defined(*messages.Pickle, *messages.PickleStep, *formatters.StepDefinition) {
}

func (f *reportFormatter) Failed(pickle *messages.Pickle, step *messages.PickleStep, _ *formatters.StepDefinition, err error) {
	f.record(pickle, step, "failed", err)
}

func (f *reportFormatter) Passed(pickle *messages.Pickle, step *messages.PickleStep, _ *formatters.StepDefinition) {
	f.record(pickle, step, "passed", nil)
}

func (f *reportFormatter) Skipped(pickle *messages.Pickle, step *messages.PickleStep, _ *formatters.StepDefinition) {
	f.record(pickle, step, "skipped", nil)
}

func (f *reportFormatter) Undefined(pickle *messages.Pickle, step *messages.PickleStep, _ *formatters.StepDefinition) {
	f.record(pickle, step, "undefined", godog.ErrUndefined)
}

func (f *reportFormatter) Pending(pickle *messages.Pickle, step *messages.PickleStep, _ *formatters.StepDefinition) {
	f.record(pickle, step, "pending", godog.ErrPending)
}

func (f *reportFormatter) Ambiguous(pickle *messages.Pickle, step *messages.PickleStep, _ *formatters.StepDefinition, err error) {
	f.record(pickle, step, "ambiguous", err)
}

func (f *reportFormatter) Summary() {}

func (f *reportFormatter) record(pickle *messages.Pickle, step *messages.PickleStep, status string, err error) {
	if f.collector == nil || pickle == nil || step == nil {
		return
	}
	event := stepEvent{text: step.Text, status: status, err: err}
	f.collector.mu.Lock()
	defer f.collector.mu.Unlock()
	if feature, ok := f.collector.features[pickle.Uri]; ok {
		for _, nodeID := range step.AstNodeIds {
			if line, exists := feature.stepLineByID[nodeID]; exists {
				event.line = line
				break
			}
		}
	}
	if event.line == 0 && pickle.Location != nil {
		event.line = pickle.Location.Line
	}
	f.collector.stepEvents[pickle.Id] = append(f.collector.stepEvents[pickle.Id], event)
}

func (c *formatterCollector) scenario(id string) scenarioEvents {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pickleIDs[id]
}

func (c *formatterCollector) steps(id string) []stepEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]stepEvent(nil), c.stepEvents[id]...)
}

func (c *formatterCollector) feature(uri string) featureEvents {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.features[uri]
}

func collectFeatureStepLines(feature *messages.Feature, lines map[string]int64) {
	collectSteps := func(steps []*messages.Step) {
		for _, step := range steps {
			if step != nil && step.Location != nil {
				lines[step.Id] = step.Location.Line
			}
		}
	}
	collectScenario := func(scenario *messages.Scenario) {
		if scenario != nil {
			collectSteps(scenario.Steps)
		}
	}
	collectRule := func(rule *messages.Rule) {
		if rule == nil {
			return
		}
		for _, child := range rule.Children {
			if child == nil {
				continue
			}
			if child.Background != nil {
				collectSteps(child.Background.Steps)
			}
			collectScenario(child.Scenario)
		}
	}
	for _, child := range feature.Children {
		if child == nil {
			continue
		}
		if child.Background != nil {
			collectSteps(child.Background.Steps)
		}
		collectScenario(child.Scenario)
		collectRule(child.Rule)
	}
}
