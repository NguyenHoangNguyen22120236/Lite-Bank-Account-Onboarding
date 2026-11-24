package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
)

// Embedding the SDK interface lets these tests implement only the two calls
// startup may make. Any accidental extra SDK call fails immediately.
type startupClient struct {
	client.Client
	execute  func(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error)
	describe func(context.Context, string, string) (*client.WorkflowExecutionDescription, error)
}

func (c startupClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return c.execute(ctx, options, workflow, args...)
}

func (c startupClient) DescribeWorkflow(ctx context.Context, id, run string) (*client.WorkflowExecutionDescription, error) {
	return c.describe(ctx, id, run)
}

type startupRun struct {
	client.WorkflowRun
	id, run string
}

func (r startupRun) GetID() string    { return r.id }
func (r startupRun) GetRunID() string { return r.run }

func TestStartupOptionsAndDeadline(t *testing.T) {
	for _, earlierDeadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "three second cap", true: "earlier caller deadline"}[earlierDeadline], func(t *testing.T) {
			ctx := context.Background()
			if earlierDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
			}
			id := uuid.NewString()
			before := time.Now()
			calls := 0
			c := startupClient{execute: func(gotCtx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
				calls++
				deadline, ok := gotCtx.Deadline()
				if !ok || deadline.After(time.Now().Add(3*time.Second)) || deadline.Before(before) {
					t.Fatal("startup lacks a bounded deadline")
				}
				if callerDeadline, ok := ctx.Deadline(); ok && deadline.After(callerDeadline) {
					t.Fatal("caller deadline extended")
				}
				if options.ID != id || options.TaskQueue != "bank-onboarding" || options.WorkflowIDReusePolicy != enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE || options.WorkflowIDConflictPolicy != enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING || !options.WorkflowExecutionErrorWhenAlreadyStarted {
					t.Fatalf("unsafe start options: %+v", options)
				}
				if options.RetryPolicy != nil || options.WorkflowExecutionTimeout != 0 || options.WorkflowRunTimeout != 0 {
					t.Fatal("startup configured workflow retries or lifetime timeout")
				}
				if workflow != "OnboardingWorkflow" || len(args) != 1 || args[0] != id {
					t.Fatal("workflow must receive only application ID")
				}
				return startupRun{id: id, run: "run-1"}, nil
			}}
			got, err := New(c, "bank-onboarding").EnsureStarted(ctx, id, id)
			if err != nil || got != service.WorkflowStartEstablished || calls != 1 {
				t.Fatalf("got %v, %v; calls=%d", got, err, calls)
			}
		})
	}
}

func TestStartupClosedDuplicateClassification(t *testing.T) {
	for _, tc := range []struct {
		status enumspb.WorkflowExecutionStatus
		want   service.WorkflowStartOutcome
	}{
		{enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, service.WorkflowStartEstablished},
		{enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, service.WorkflowStartCompleted},
		{enumspb.WORKFLOW_EXECUTION_STATUS_FAILED, service.WorkflowStartClosedUnsuccessfully},
		{enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED, service.WorkflowStartClosedUnsuccessfully},
		{enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, service.WorkflowStartClosedUnsuccessfully},
		{enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT, service.WorkflowStartClosedUnsuccessfully},
	} {
		t.Run(tc.status.String(), func(t *testing.T) {
			id := uuid.NewString()
			var startupDeadline time.Time
			starts, describes := 0, 0
			c := startupClient{
				execute: func(ctx context.Context, _ client.StartWorkflowOptions, _ interface{}, _ ...interface{}) (client.WorkflowRun, error) {
					starts++
					startupDeadline, _ = ctx.Deadline()
					return nil, &serviceerror.WorkflowExecutionAlreadyStarted{RunId: "existing-run"}
				},
				describe: func(ctx context.Context, workflowID, runID string) (*client.WorkflowExecutionDescription, error) {
					describes++
					deadline, ok := ctx.Deadline()
					if !ok || deadline.After(startupDeadline) || workflowID != id || runID != "existing-run" {
						t.Fatal("describe must use exact run and remaining startup budget")
					}
					return &client.WorkflowExecutionDescription{RawResponse: &workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{Status: tc.status}}}, nil
				},
			}
			got, err := New(c, "bank-onboarding").EnsureStarted(context.Background(), id, id)
			if err != nil || got != tc.want || starts != 1 || describes != 1 {
				t.Fatalf("got %v %v; starts=%d describes=%d", got, err, starts, describes)
			}
		})
	}
}

func TestStartupErrorsDoNotRetry(t *testing.T) {
	for _, describeFailure := range []bool{false, true} {
		for _, tc := range []struct {
			name        string
			cause, want error
		}{
			{"unavailable", serviceerror.NewUnavailable("synthetic-sensitive-detail"), service.ErrWorkflowUnavailable},
			{"deadline", context.DeadlineExceeded, context.DeadlineExceeded},
			{"canceled", context.Canceled, context.Canceled},
		} {
			t.Run(tc.name+map[bool]string{false: "/start", true: "/describe"}[describeFailure], func(t *testing.T) {
				starts, describes := 0, 0
				c := startupClient{
					execute: func(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error) {
						starts++
						if describeFailure {
							return nil, &serviceerror.WorkflowExecutionAlreadyStarted{RunId: "existing-run"}
						}
						return nil, tc.cause
					},
					describe: func(context.Context, string, string) (*client.WorkflowExecutionDescription, error) {
						describes++
						return nil, tc.cause
					},
				}
				id := uuid.NewString()
				_, err := New(c, "bank-onboarding").EnsureStarted(context.Background(), id, id)
				if !errors.Is(err, tc.want) || starts != 1 || describes != map[bool]int{false: 0, true: 1}[describeFailure] {
					t.Fatalf("error=%v starts=%d describes=%d", err, starts, describes)
				}
			})
		}
	}
}
