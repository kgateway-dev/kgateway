package spec

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"time"
)

var _ = Describe("ScenarioRunner", func() {

	var (
		ctx            context.Context
		scenarioRunner *ScenarioRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		scenarioRunner = NewScenarioRunner().
			WithProgressWriter(GinkgoWriter).
			WithTimeout(time.Minute)
	})

	It("returns error on invalid setup", func() {
		s := &testScenario{
			initResources: func(ctx context.Context) error {
				return eris.Errorf("Failed to initialize resources")
			},
		}

		err := scenarioRunner.RunScenario(ctx, s)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("Failed to initialize resources"))
	})

	It("returns error on invalid cleanup", func() {
		s := &testScenario{
			finalizeResources: func(ctx context.Context) error {
				return eris.Errorf("Failed to finalize resources")
			},
		}

		err := scenarioRunner.RunScenario(ctx, s)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("Failed to finalize resources"))
	})

	FIt("cancels long running scenario", func() {
		s := &testScenario{
			initResources: func(ctx context.Context) error {
				// block for forever, unless the context is cancelled
				select {}
			},
		}

		err := scenarioRunner.WithTimeout(time.Second).RunScenario(ctx, s)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError("Failed to finalize resources"))
	})
})

var _ Scenario = new(testScenario)

// testScenario is used only in this file, to validate that the ScenarioRunner operates as expected
type testScenario struct {
	name              string
	initResources     func(ctx context.Context) error
	finalizeResources func(ctx context.Context) error
}

func (t *testScenario) Name() string {
	if t.name != "" {
		return t.name
	}
	return "test-scenario"
}

func (t *testScenario) InitializeResources() func(ctx context.Context) error {
	if t.initResources != nil {
		return t.initResources
	}
	return func(ctx context.Context) error {
		return nil
	}
}

func (t *testScenario) WaitForInitialized() ScenarioAssertion {
	return func(ctx context.Context) {
		// do nothing
	}
}

func (t *testScenario) Assertion() ScenarioAssertion {
	return func(ctx context.Context) {
		// do nothing
	}
}

func (t *testScenario) ChildScenario() Scenario {
	return nil
}

func (t *testScenario) FinalizeResources() func(ctx context.Context) error {
	if t.finalizeResources != nil {
		return t.finalizeResources
	}
	return func(ctx context.Context) error {
		return nil
	}
}

func (t *testScenario) WaitForFinalized() ScenarioAssertion {
	return func(ctx context.Context) {
		// do nothing
	}
}
