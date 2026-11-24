package workflow

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func TestOnboardingWaitsDurablyUntilCanceled(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterDelayedCallback(func() {
		if env.IsWorkflowCompleted() {
			t.Fatal("placeholder completed instead of waiting durably")
		}
		env.CancelWorkflow()
	}, time.Hour)
	// No activity implementation or mock is registered: scheduling any business
	// activity is outside this workflow's approved scope.
	env.ExecuteWorkflow(OnboardingWorkflow, uuid.NewString())
	if !env.IsWorkflowCompleted() || !temporal.IsCanceledError(env.GetWorkflowError()) {
		t.Fatalf("expected durable wait followed by cancellation, got %v", env.GetWorkflowError())
	}
}
