package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConnectAddsBearerCredentialToEveryRequest(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-gateway", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var authenticated atomic.Int64
	httpServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-credential" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		authenticated.Add(1)
		streamable.ServeHTTP(writer, request)
	}))
	defer httpServer.Close()

	connection, err := connect(context.Background(), httpServer.URL, "test-credential", "test", httpServer.Client(), httpServer.CloseClientConnections)
	if err != nil {
		t.Fatal(err)
	}
	for tool, listErr := range connection.Session.Tools(context.Background(), nil) {
		if listErr != nil {
			t.Fatal(listErr)
		}
		if tool.Name != "ping" {
			t.Fatalf("tool name = %q, want ping", tool.Name)
		}
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	if authenticated.Load() < 2 {
		t.Fatalf("authenticated requests = %d, want at least 2", authenticated.Load())
	}
}

func TestConnectRejectsCredentialInURL(t *testing.T) {
	_, err := Connect(context.Background(), "https://user:secret-value@example.com/mcp", "credential", "test")
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unexpected error: %v", err)
	}
}
