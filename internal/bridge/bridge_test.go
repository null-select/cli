package bridge

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/null-select/cli/internal/remote"
)

type echoInput struct {
	Value string `json:"value" jsonschema:"the value to return"`
}

type echoOutput struct {
	Value string `json:"value"`
}

func TestProxyMirrorsAndForwardsTools(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upstreamServer := mcp.NewServer(&mcp.Implementation{Name: "test-gateway", Version: "1.0.0"}, nil)
	mcp.AddTool(upstreamServer, &mcp.Tool{Name: "echo", Description: "Return the supplied value."}, func(_ context.Context, _ *mcp.CallToolRequest, input echoInput) (*mcp.CallToolResult, echoOutput, error) {
		return nil, echoOutput{Value: input.Value}, nil
	})
	upstreamClientTransport, upstreamServerTransport := mcp.NewInMemoryTransports()
	upstreamDone := make(chan error, 1)
	go func() { upstreamDone <- upstreamServer.Run(ctx, upstreamServerTransport) }()
	upstreamClient := mcp.NewClient(&mcp.Implementation{Name: "bridge-test", Version: "test"}, nil)
	upstreamSession, err := upstreamClient.Connect(ctx, upstreamClientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}

	proxy, err := newProxy(ctx, &remote.Connection{Session: upstreamSession}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	localClientTransport, localServerTransport := mcp.NewInMemoryTransports()
	proxyDone := make(chan error, 1)
	go func() { proxyDone <- proxy.Serve(ctx, localServerTransport) }()
	localClient := mcp.NewClient(&mcp.Implementation{Name: "local-client", Version: "test"}, nil)
	localSession, err := localClient.Connect(ctx, localClientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer localSession.Close()

	toolCount := 0
	for tool, listErr := range localSession.Tools(ctx, nil) {
		if listErr != nil {
			t.Fatal(listErr)
		}
		toolCount++
		if tool.Name != "echo" || tool.Description != "Return the supplied value." {
			t.Fatalf("unexpected mirrored tool: %#v", tool)
		}
	}
	if toolCount != 1 {
		t.Fatalf("tool count = %d, want 1", toolCount)
	}
	result, err := localSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"value": "through-the-gateway"},
	})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["value"] != "through-the-gateway" {
		t.Fatalf("unexpected tool result: %#v", result.StructuredContent)
	}
}
