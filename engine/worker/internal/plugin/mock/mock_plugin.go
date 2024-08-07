package mock

import (
	"context"

	"github.com/ovh/cds/engine/worker/internal/plugin"
	"github.com/ovh/cds/engine/worker/pkg/workerruntime"
	"github.com/ovh/cds/sdk"
)

type MockFactory struct {
	Result []string
	Index  int
}

func (pf *MockFactory) NewClient(ctx context.Context, wk workerruntime.Runtime, pluginType string, pluginName string, inputManagement string, env map[string]string) (plugin.Client, error) {
	c, err := NewMockClient(pf.Result[pf.Index])
	pf.Index++
	return c, err
}

type MockClient struct {
	Result string
}

func (m MockClient) Close(ctx context.Context) {
	return
}

func (m *MockClient) Run(ctx context.Context, opts map[string]string) *plugin.Result {
	return &plugin.Result{Status: m.Result}
}

func (m *MockClient) GetPostAction() *sdk.PluginPost {
	return nil
}

func NewMockClient(status string) (plugin.Client, error) {
	return &MockClient{
		Result: status,
	}, nil
}
