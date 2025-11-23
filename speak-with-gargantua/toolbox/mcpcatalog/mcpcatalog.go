package mcpcatalog

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/mcp"
)

type ListToolsInput struct{}

func GetToolsList(ctx context.Context, mcpClient *mcp.GenkitMCPClient) ([]ai.ToolRef, error) {

	g := genkit.Init(ctx, genkit.WithPlugins(&openai.OpenAI{
		APIKey: "I💙DockerModelRunner",
	}))

	toolsList, err := mcpClient.GetActiveTools(ctx, g)

	if err != nil {
		fmt.Println("😡 Error getting the tools list:", err)
		return nil, err
		//os.Exit(1)
	}

	// Keep MCP tools as ai.Tool (don't convert to ToolRef)
	// This preserves the RunRaw() method needed for execution
	toolRefs := make([]ai.ToolRef, 0, len(toolsList)+1)

	// Add MCP tools directly (ai.Tool implements ai.ToolRef)
	for _, tool := range toolsList {
		toolRefs = append(toolRefs, tool)
	}

	return toolRefs, nil
}
