package workflow

import temporalworkflow "go.temporal.io/sdk/workflow"

const Name = "OnboardingWorkflow"

// OnboardingWorkflow deliberately performs no business work in PR 06.
// Await records no busy loop or external I/O and survives worker restarts.
func OnboardingWorkflow(ctx temporalworkflow.Context, applicationID string) error {
	return temporalworkflow.Await(ctx, func() bool { return false })
}
