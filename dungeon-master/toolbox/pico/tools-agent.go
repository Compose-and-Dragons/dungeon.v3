package pico

import (
	"context"
	"dungeon-master/toolbox/conversion"
	"dungeon-master/toolbox/env"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type ToolsAIAgent struct {
	ctx                context.Context
	Name               string
	SystemInstructions string
	ModelID            string
	Messages           []*ai.Message

	Config *openai.ChatCompletionNewParams

	ToolsIndex []ai.ToolRef

	toolCallingFlow *core.Flow[*ToolCallsRequest, ToolCallsResult, struct{}]

	genKitInstance *genkit.Genkit
}

func (agent *ToolsAIAgent) GetName() string {
	return agent.Name
}
func (agent *ToolsAIAgent) GetMessages() []*ai.Message {
	return agent.Messages
}

func (agent *ToolsAIAgent) DisplayConversationHistory() {
	fmt.Println("🧾 Conversation History for agent:", agent.Name)
	for i, msg := range agent.Messages {
		jsonBytes, err := json.MarshalIndent(msg, "", "  ")
		if err != nil {
			log.Printf("Failed to marshal message %d: %v\n", i, err)
			continue
		}
		fmt.Printf("Message %d:\n%s\n", i+1, string(jsonBytes))
	}
}

func NewToolsAIAgent(ctx context.Context, name, systemInstructions, modelID, engineURL string, config *openai.ChatCompletionNewParams, toolsIndex []ai.ToolRef) *ToolsAIAgent {
	oaiPlugin := &oai.OpenAI{
		APIKey: "I💙DockerModelRunner",
		Opts: []option.RequestOption{
			option.WithBaseURL(engineURL),
		},
	}

	genKitInstance := genkit.Init(ctx, genkit.WithPlugins(oaiPlugin))

	agent := &ToolsAIAgent{
		Name:               name,
		SystemInstructions: systemInstructions,
		ModelID:            modelID,
		Messages:           []*ai.Message{},
		Config:             config,
		ToolsIndex:         toolsIndex,

		ctx:            ctx,
		genKitInstance: genKitInstance,
	}

	initializeToolsFlow(agent)

	return agent

}

type ToolCallsRequest struct {
	Prompt string `json:"prompt"`
}
type ToolCallsResult struct {
	Text string              `json:"text"`
	List []map[string]string `json:"list"`
}

func initializeToolsFlow(agent *ToolsAIAgent) {

	// STEP 1: Define tool-calling flow

	toolCallingFlow := genkit.DefineFlow(agent.genKitInstance, agent.Name+"-tool-calling-flow",
		func(ctx context.Context, req *ToolCallsRequest) (ToolCallsResult, error) {

			// STEP 2: Initialize loop control variables
			stopped := false           // Controls the conversation loop
			lastAssistantMessage := "" // Final AI message

			//totalOfToolsCalls := 0
			toolCallsResults := []map[string]string{}

			history := []*ai.Message{}
			// STEP 3: Start the conversation loop
			// To avoid repeating the first user message in the history
			// we add it here before entering the loop and using prompt
			history = append(history, ai.NewUserTextMessage(req.Prompt))
			// TODO: use agent.Messages as initial history?

			for !stopped { // BEGIN: of loop

				resp, err := genkit.Generate(ctx, agent.genKitInstance,
					ai.WithModelName("openai/"+agent.ModelID),
					ai.WithSystem(agent.SystemInstructions),
					// WithMessages sets the messages. These messages will be sandwiched between the system and user prompts.
					// ai.WithMessages(
					// 	agent.Messages...,
					// ),
					ai.WithMessages(
						history...,
					),
					//ai.WithPrompt(req.Prompt), // NOTE: do not add the prompt again
					ai.WithTools(
						agent.ToolsIndex...,
					),
					ai.WithToolChoice(ai.ToolChoiceAuto),
					ai.WithReturnToolRequests(true),
				)
				if err != nil {
					return ToolCallsResult{}, err
				}

				// We do not use parallel tool calls
				toolRequests := resp.ToolRequests()
				if len(toolRequests) == 0 {
					// No tool requests, we are done
					stopped = true // Exit the loop
					lastAssistantMessage = resp.Text()
					break // Exit the loop now
				}
				// IMPORTANT: Add the assistant's message with tool requests to history
				// This ensures the model knows it already proposed these tools
				// history = append(history, resp.Message)
				history = append(history, resp.Message)

				for _, req := range toolRequests {

					var tool ai.Tool
					// tool = genkit.LookupTool(agent.genKitInstance, req.Name)

					for _, t := range agent.ToolsIndex {
						if t.Name() == req.Name {
							// Try to convert ToolRef to Tool
							if toolImpl, ok := t.(ai.Tool); ok {
								tool = toolImpl
								// ✅ Successfully converted to ai.Tool"
								break
							}
							// else: ❌ Failed to convert ToolRef to ai.Tool")
						}
					}

					displayToolRequets(req)

					if tool == nil {
						log.Fatalf("tool %q not found", req.Name)
					}

					execConfirmation := func() {
						var response string
						for {
							// fmt.Printf("Do you want to execute tool %q? (y/n/q): ", req.Name)

							fmt.Fprintf(os.Stdout, "Do you want to execute tool %q? (y/n/q): ", req.Name)
							os.Stdout.Sync() // Force flush stdout buffer
							_, err := fmt.Scanln(&response)
							if err != nil {
								fmt.Println("Error reading input:", err)
								continue
							}
							response = strings.ToLower(strings.TrimSpace(response))

							switch response {
							case "q":
								fmt.Println("Exiting the program.")
								stopped = true
								return
							case "y":
								output, err := tool.RunRaw(ctx, req.Input)
								if err != nil {
									log.Fatalf("tool %q execution failed: %v", tool.Name(), err)
								}
								displayToolCallResult(output)

								part := ai.NewToolResponsePart(&ai.ToolResponse{
									Name:   req.Name,
									Ref:    req.Ref,
									Output: output,
								})

								history = append(history, ai.NewMessage(ai.RoleTool, nil, part))

								toolCallOutput := castToToolOutput(output)
								if len(toolCallOutput.Content) > 0 {
									toolCallsResults = append(toolCallsResults, map[string]string{
										tool.Name(): toolCallOutput.Content[0].Text,
									})
								} else {
									// Fallback: use the raw output as a string
									outputStr := fmt.Sprintf("%v", output)
									toolCallsResults = append(toolCallsResults, map[string]string{
										tool.Name(): outputStr,
									})
								}

								return
							case "n":
								fmt.Println("⏩ Skipping tool execution.", req.Name, req.Ref)

								// Add tool response indicating the tool was not executed
								part := ai.NewToolResponsePart(&ai.ToolResponse{
									Name:   req.Name,
									Ref:    req.Ref,
									Output: map[string]any{"error": "Tool execution cancelled by user"},
								})
								history = append(history, ai.NewMessage(ai.RoleTool, nil, part))

								return
							default:
								fmt.Println("Please enter 'y' or 'n'.")
								continue
							}

						}

					}
					execConfirmation()

				}

			} // END: of loop
			return ToolCallsResult{
				Text: lastAssistantMessage,
				List: toolCallsResults,
			}, nil
		})
	agent.toolCallingFlow = toolCallingFlow
}

func (agent *ToolsAIAgent) RunToolCalls(prompt string) (ToolCallsResult, error) {
	resp, err := agent.toolCallingFlow.Run(agent.ctx, &ToolCallsRequest{
		Prompt: prompt,
	})
	if err != nil {
		return ToolCallsResult{}, err
	}
	return resp, nil
}

func displayToolRequets(toolRequest *ai.ToolRequest) {
	jsonInput, err := json.Marshal(toolRequest.Input)
	if err != nil {
		fmt.Println("🛠️ Tool request:", toolRequest.Name, toolRequest.Ref, toolRequest.Input)
	}
	fmt.Println("🛠️ Tool request:", toolRequest.Name, "args:", string(jsonInput))
}

type ContentItem struct {
	Text string `json:"text"`
	Type string `json:"type"`
}
type ToolOutput struct {
	Content []ContentItem `json:"content"`
}

func castToToolOutput(output any) ToolOutput {
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		log.Printf("Failed to marshal tool output: %v\n", err)
		return ToolOutput{
			Content: []ContentItem{{
				Text: err.Error(),
				Type: "text",
			}},
		}
	}
	var result ToolOutput
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		log.Printf("Failed to unmarshal tool output: %v\n", err)
		return ToolOutput{
			Content: []ContentItem{{
				Text: err.Error(),
				Type: "text",
			}},
		}
	}
	return result
}

func displayToolCallResult(output any) {
	shouldIDisplay := env.GetEnvOrDefault("LOG_MESSAGES", "false")

	if conversion.StringToBool(shouldIDisplay) {

		jsonBytes, err := json.Marshal(output)
		if err != nil {
			fmt.Println("🤖 Tool output:", output)
			return
		}
		var result ToolOutput
		json.Unmarshal(jsonBytes, &result)
		if len(result.Content) > 0 {
			fmt.Println("🤖 Tool output:", result.Content[0].Text)
		} else {
			fmt.Println("🤖 Tool output:", output)
		}
	}
}

func (agent *ToolsAIAgent) AskQuestion(question string) (string, error) {
	return "", nil
}

func (agent *ToolsAIAgent) AskQuestionStream(question string, callback func(string) error) (string, error) {
	return "", nil
}

func (agent *ToolsAIAgent) DirectExecuteTool(input ai.ToolRequest) (string, error) {
	var tool ai.Tool
	// tool = genkit.LookupTool(agent.genKitInstance, toolName)

	for _, t := range agent.ToolsIndex {
		if t.Name() == input.Name {
			// Try to convert ToolRef to Tool
			if toolImpl, ok := t.(ai.Tool); ok {
				tool = toolImpl
				// ✅ Successfully converted to ai.Tool"
				break
			}
			// else: ❌ Failed to convert ToolRef to ai.Tool")
		}
	}

	if tool == nil {
		log.Fatalf("tool %q not found", input.Name)
	}

	output, err := tool.RunRaw(agent.ctx, input.Input)
	if err != nil {
		log.Fatalf("tool %q execution failed: %v", tool.Name(), err)
	}

	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	return string(jsonOutput), nil
}
