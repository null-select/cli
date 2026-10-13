package doctor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/null-select/cli/internal/remote"
)

const maximumTools = 500

type Report struct {
	Endpoint        string `json:"endpoint"`
	Connected       bool   `json:"connected"`
	ProtocolVersion string `json:"protocol_version"`
	ServerName      string `json:"server_name"`
	ServerVersion   string `json:"server_version"`
	ToolCount       int    `json:"tool_count"`
	CheckedAt       string `json:"checked_at"`
}

func Check(ctx context.Context, endpoint, credential, version string, now time.Time) (Report, error) {
	connection, err := remote.Connect(ctx, endpoint, credential, version)
	if err != nil {
		return Report{}, err
	}
	defer connection.Close()

	initialize := connection.Session.InitializeResult()
	if initialize == nil || initialize.ProtocolVersion == "" {
		return Report{}, errors.New("MCP endpoint returned no negotiated protocol version")
	}
	report := Report{
		Endpoint:        endpoint,
		Connected:       true,
		ProtocolVersion: initialize.ProtocolVersion,
		CheckedAt:       now.UTC().Format(time.RFC3339),
	}
	if initialize.ServerInfo != nil {
		report.ServerName = initialize.ServerInfo.Name
		report.ServerVersion = initialize.ServerInfo.Version
	}
	for tool, listErr := range connection.Session.Tools(ctx, nil) {
		if listErr != nil {
			return Report{}, fmt.Errorf("list gateway tools: %w", listErr)
		}
		if tool == nil || tool.Name == "" {
			return Report{}, errors.New("gateway returned a tool without a name")
		}
		report.ToolCount++
		if report.ToolCount > maximumTools {
			return Report{}, fmt.Errorf("gateway exposes more than the supported limit of %d tools", maximumTools)
		}
	}
	return report, nil
}
