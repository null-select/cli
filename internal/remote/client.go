package remote

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maximumEventBytes = 8 << 20

type Connection struct {
	Session   *mcp.ClientSession
	closeIdle func()
}

func Connect(ctx context.Context, endpoint, credential, version string) (*Connection, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("endpoint must be an absolute HTTPS URL without user information, query parameters, or a fragment")
	}
	if credential == "" {
		return nil, errors.New("credential is empty")
	}

	baseTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	httpClient := &http.Client{Transport: baseTransport}
	return connect(ctx, parsed.String(), credential, version, httpClient, baseTransport.CloseIdleConnections)
}

func connect(
	ctx context.Context,
	endpoint, credential, version string,
	baseClient *http.Client,
	closeIdle func(),
) (*Connection, error) {
	if baseClient == nil || baseClient.Transport == nil {
		return nil, errors.New("HTTP client transport is not configured")
	}
	httpClient := *baseClient
	httpClient.Transport = &bearerTransport{base: baseClient.Transport, credential: credential}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nullselect-cli",
		Title:   "null.select CLI",
		Version: version,
	}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &httpClient,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
		MaxEventSize:         maximumEventBytes,
	}, nil)
	if err != nil {
		if closeIdle != nil {
			closeIdle()
		}
		return nil, fmt.Errorf("connect to MCP endpoint: %w", err)
	}
	return &Connection{Session: session, closeIdle: closeIdle}, nil
}

func (connection *Connection) Close() error {
	if connection == nil || connection.Session == nil {
		return nil
	}
	err := connection.Session.Close()
	if connection.closeIdle != nil {
		connection.closeIdle()
	}
	return err
}

type bearerTransport struct {
	base       http.RoundTripper
	credential string
}

func (transport *bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+transport.credential)
	return transport.base.RoundTrip(clone)
}
