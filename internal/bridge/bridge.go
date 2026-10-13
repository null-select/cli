package bridge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/null-select/cli/internal/remote"
)

const maximumTools = 500

type Proxy struct {
	connection *remote.Connection
	server     *mcp.Server
	closeOnce  sync.Once
	closeErr   error
}

func New(ctx context.Context, endpoint, credential, version string) (*Proxy, error) {
	connection, err := remote.Connect(ctx, endpoint, credential, version)
	if err != nil {
		return nil, err
	}
	proxy, err := newProxy(ctx, connection, version)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}

func newProxy(ctx context.Context, connection *remote.Connection, version string) (*Proxy, error) {
	failed := true
	defer func() {
		if failed {
			_ = connection.Close()
		}
	}()

	initialize := connection.Session.InitializeResult()
	if initialize == nil || initialize.ProtocolVersion == "" {
		return nil, errors.New("MCP endpoint returned no negotiated protocol version")
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "nullselect-proxy",
		Title:       "null.select gateway",
		Description: "Local stdio bridge to the null.select managed MCP gateway.",
		Version:     version,
	}, &mcp.ServerOptions{
		Instructions:              initialize.Instructions,
		SupportedProtocolVersions: []string{initialize.ProtocolVersion},
	})

	seen := make(map[string]struct{})
	count := 0
	for tool, listErr := range connection.Session.Tools(ctx, nil) {
		if listErr != nil {
			return nil, fmt.Errorf("list gateway tools: %w", listErr)
		}
		if tool == nil || tool.Name == "" {
			return nil, errors.New("gateway returned a tool without a name")
		}
		if _, exists := seen[tool.Name]; exists {
			return nil, fmt.Errorf("gateway returned duplicate tool %q", tool.Name)
		}
		count++
		if count > maximumTools {
			return nil, fmt.Errorf("gateway exposes more than the supported limit of %d tools", maximumTools)
		}
		seen[tool.Name] = struct{}{}
		upstreamTool := tool
		server.AddTool(upstreamTool, func(callCtx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			params := request.Params
			return connection.Session.CallTool(callCtx, &mcp.CallToolParams{
				Meta:           params.Meta,
				Name:           upstreamTool.Name,
				Arguments:      params.Arguments,
				InputResponses: params.InputResponses,
				RequestState:   params.RequestState,
			})
		})
	}

	failed = false
	return &Proxy{connection: connection, server: server}, nil
}

func (proxy *Proxy) Serve(ctx context.Context, transport mcp.Transport) error {
	if proxy == nil || proxy.server == nil {
		return errors.New("proxy is not initialized")
	}
	return proxy.server.Run(ctx, transport)
}

func (proxy *Proxy) Close() error {
	if proxy == nil {
		return nil
	}
	proxy.closeOnce.Do(func() {
		proxy.closeErr = proxy.connection.Close()
	})
	return proxy.closeErr
}
