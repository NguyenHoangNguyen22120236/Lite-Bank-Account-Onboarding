package workflow

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/application/service"
	"github.com/NguyenHoangNguyen22120236/Lite-Bank-Account-Onboarding/internal/platform/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

// recordingTemporalClient forwards every call to the real Temporal client.
// It records the returned handles because EnsureStarted exposes only an outcome.
type recordingTemporalClient struct {
	client.Client
	runs []client.WorkflowRun
}

func (c *recordingTemporalClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	run, err := c.Client.ExecuteWorkflow(ctx, options, workflow, args...)
	if err == nil {
		c.runs = append(c.runs, run)
	}
	return run, err
}

func TestIntegrationStartupReusesRunningWorkflow(t *testing.T) {
	require.NoError(t, config.LoadDotEnv("../../../.env"))
	// Explicit opt-in only: do not contact Temporal during ordinary unit tests.
	address := os.Getenv("TEST_TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("TEST_TEMPORAL_ADDRESS is not set")
	}
	namespace := os.Getenv("TEST_TEMPORAL_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, err := client.DialContext(ctx, client.Options{HostPort: address, Namespace: namespace})
	require.True(t, err == nil, "cannot connect to test Temporal server (connection details omitted)")
	t.Cleanup(c.Close)

	id := uuid.NewString()
	// An unpolled, unique queue keeps the execution running without depending on
	// the local worker or its deployed workflow implementation. No worker is needed
	// for the server to accept startup and enforce workflow ID policies.
	recording := &recordingTemporalClient{Client: c}
	starter := New(recording, "startup-integration-"+id)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		// The UUID is owned exclusively by this test, including uncertain starts.
		err := c.TerminateWorkflow(cleanupCtx, id, "", "startup integration test cleanup")
		var notFound *serviceerror.NotFound
		assert.False(t, err != nil && !errors.As(err, &notFound), "could not terminate test workflow (details omitted)")
	})

	for attempt := 1; attempt <= 2; attempt++ {
		outcome, err := starter.EnsureStarted(ctx, id, id)
		require.NoError(t, err)
		require.Equal(t, service.WorkflowStartEstablished, outcome)
	}
	require.Len(t, recording.runs, 2)
	first, second := recording.runs[0], recording.runs[1]
	require.Equal(t, id, first.GetID())
	require.Equal(t, id, second.GetID())
	require.NotEmpty(t, first.GetRunID())
	require.Equal(t, second.GetRunID(), first.GetRunID())
	response, err := c.DescribeWorkflow(ctx, id, first.GetRunID())
	require.True(t, err == nil, "could not describe test workflow (details omitted)")
	require.NotNil(t, response)
	require.Equal(t, enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, response.RawResponse.GetWorkflowExecutionInfo().GetStatus())
}
