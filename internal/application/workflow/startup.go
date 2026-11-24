package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	workflowdefinition "github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/workflow"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

type Client interface {
	ExecuteWorkflow(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
	DescribeWorkflow(context.Context, string, string) (*client.WorkflowExecutionDescription, error)
}

// Starter is Main BE's client adapter, not worker workflow orchestration.
type Starter struct {
	client    Client
	taskQueue string
}

var _ service.WorkflowStarter = (*Starter)(nil)

func New(c Client, taskQueue string) *Starter { return &Starter{client: c, taskQueue: taskQueue} }

func (s *Starter) EnsureStarted(ctx context.Context, applicationID, workflowID string) (service.WorkflowStartOutcome, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: workflowID, TaskQueue: s.taskQueue,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowIDConflictPolicy:                 enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, workflowdefinition.Name, applicationID)
	if err == nil {
		return service.WorkflowStartEstablished, nil
	}
	var duplicate *serviceerror.WorkflowExecutionAlreadyStarted
	if !errors.As(err, &duplicate) {
		return 0, startupError(ctx, err)
	}
	if duplicate.RunId == "" {
		return 0, service.ErrWorkflowUnavailable
	}
	response, err := s.client.DescribeWorkflow(ctx, workflowID, duplicate.RunId)
	if err != nil {
		return 0, startupError(ctx, err)
	}
	if response == nil || response.RawResponse == nil || response.RawResponse.WorkflowExecutionInfo == nil {
		return 0, service.ErrWorkflowUnavailable
	}
	switch response.RawResponse.WorkflowExecutionInfo.Status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return service.WorkflowStartEstablished, nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return service.WorkflowStartCompleted, nil
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED, enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED, enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return service.WorkflowStartClosedUnsuccessfully, nil
	default:
		return 0, service.ErrWorkflowUnavailable
	}
}

func startupError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var canceled *serviceerror.Canceled
	var deadline *serviceerror.DeadlineExceeded
	var unavailable *serviceerror.Unavailable
	var internal *serviceerror.Internal
	var exhausted *serviceerror.ResourceExhausted
	var notFound *serviceerror.NotFound
	switch {
	case errors.As(err, &canceled):
		return context.Canceled
	case errors.As(err, &deadline):
		return context.DeadlineExceeded
	case errors.As(err, &unavailable), errors.As(err, &internal), errors.As(err, &exhausted), errors.As(err, &notFound):
		return service.ErrWorkflowUnavailable
	default:
		return service.ErrWorkflowStartup
	}
}
