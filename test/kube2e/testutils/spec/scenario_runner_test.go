package spec

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	"strconv"
)

var _ = Describe("ScenarioRunner", func() {

	var (
		ctx            context.Context
		scenarioRunner *ScenarioRunner
	)

	BeforeEach(func() {
		ctx = context.Background()
		scenarioRunner = NewGinkgoScenarioRunner()
	})

	It("does not return error on valid setup", func() {
		s := &testScenario{
			initResources: func(ctx context.Context) error {
				return nil
			},
			finalizeResources: func(ctx context.Context) error {
				return nil
			},
		}

		err := scenarioRunner.RunScenario(ctx, s)
		Expect(err).NotTo(HaveOccurred())
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

	It("returns error if assertion fails", func() {
		s := &testScenario{
			assertion: func(ctx context.Context) {
				Expect(1).To(Equal(2), "one does not equal two")
			},
		}

		err := scenarioRunner.RunScenario(ctx, s)
		Expect(err).To(And(
			// Prove that the error includes the description of the failing assertion
			MatchError(ContainSubstring("one does not equal")),
			// Prove that the error includes the assertion that failed
			MatchError(ContainSubstring("Expected\n    <int>: 1\nto equal\n    <int>: 2")),
		))
	})

	It("executes child scenario", func() {
		failingScenario := &testScenario{
			name: "failing",
			assertion: func(ctx context.Context) {
				Expect(1).To(Equal(2), "one does not equal two")
			},
		}
		s := &testScenario{
			name:          "parent",
			childScenario: failingScenario,
		}

		err := scenarioRunner.RunScenario(ctx, s)
		Expect(err).To(And(
			// Prove that the error includes the description of the failing assertion
			MatchError(ContainSubstring("one does not equal")),
			// Prove that the error includes the assertion that failed
			MatchError(ContainSubstring("Expected\n    <int>: 1\nto equal\n    <int>: 2")),
		))
	})

	It("returns error if max depth is exceeded ", func() {
		scenario := &testScenario{
			name: "0",
		}
		var curScenario = scenario

		// TODO: When go1.22 is introduced, use range
		for i := 1; i <= 5; i++ {
			curScenario.childScenario = &testScenario{
				name: strconv.Itoa(i),
			}
			curScenario = curScenario.childScenario
		}

		err := scenarioRunner.RunScenario(ctx, scenario)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("scenario can be nested, but 3 levels is the maximum")))
	})

})

var _ Scenario = new(testScenario)

// testScenario is used only in this file, to validate that the ScenarioRunner operates as expected
type testScenario struct {
	name              string
	initResources     func(ctx context.Context) error
	finalizeResources func(ctx context.Context) error
	assertion         func(ctx context.Context)
	childScenario     *testScenario
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

func (t *testScenario) InitializedAssertion() ScenarioAssertion {
	return func(ctx context.Context) {
		// do nothing
	}
}

func (t *testScenario) Assertion() ScenarioAssertion {
	if t.assertion != nil {
		return t.assertion
	}
	return func(ctx context.Context) {
		// do nothing
	}
}

func (t *testScenario) ChildScenario() Scenario {
	if t.childScenario != nil {
		return t.childScenario
	}
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

func (t *testScenario) FinalizedAssertion() ScenarioAssertion {
	return func(ctx context.Context) {
		// do nothing
	}
}
