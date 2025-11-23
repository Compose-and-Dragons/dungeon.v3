package pico

import (
	"context"
	"dungeon-master/toolbox/conversion"
	"dungeon-master/toolbox/env"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type ChatRequest struct {
	UserMessage string `json:"message"`
}

// Structure for final flow output
type ChatResponse struct {
	Response string `json:"response"`
}

type LocalAIAgent struct {
	ctx                context.Context
	Name               string
	SystemInstructions string
	ModelID            string
	Messages           []*ai.Message

	Config *openai.ChatCompletionNewParams

	genKitInstance *genkit.Genkit
	chatStreamFlow *core.Flow[*ChatRequest, *ChatResponse, string]
}

func (agent *LocalAIAgent) GetName() string {
	return agent.Name
}

func (agent *LocalAIAgent) GetMessages() []*ai.Message {
	return agent.Messages
}

func NewLocalAIAgent(ctx context.Context, name, systemInstructions, modelID, engineURL string, config *openai.ChatCompletionNewParams) *LocalAIAgent {
	oaiPlugin := &oai.OpenAI{
		APIKey: "I💙DockerModelRunner",
		Opts: []option.RequestOption{
			option.WithBaseURL(engineURL),
		},
	}

	genKitInstance := genkit.Init(ctx, genkit.WithPlugins(oaiPlugin))

	agent := &LocalAIAgent{
		Name:               name,
		SystemInstructions: systemInstructions,
		ModelID:            modelID,
		Messages:           []*ai.Message{},
		Config:             config,

		ctx:            ctx,
		genKitInstance: genKitInstance,
	}

	initializeChatStreamFlow(agent)

	return agent

}

func displayConversationHistory(agent *LocalAIAgent) {
	// For debugging: print conversation history
	shouldIDisplay := env.GetEnvOrDefault("LOG_MESSAGES", "false")

	if conversion.StringToBool(shouldIDisplay) {

		fmt.Println()
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println("🗒️ Conversation history:")
		for _, msg := range agent.Messages {
			content := msg.Content[0].Text
			if len(content) > 80 {
				fmt.Println("📝", msg.Role, ":", content[:80]+"...")
			} else {
				fmt.Println("📝", msg.Role, ":", content)
			}
		}
		fmt.Println(strings.Repeat("-", 50))
	}
}

func initializeChatStreamFlow(agent *LocalAIAgent) {

	chatStreamFlow := genkit.DefineStreamingFlow(agent.genKitInstance, agent.Name+"-chat-stream-flow",
		func(ctx context.Context, input *ChatRequest, callback core.StreamCallback[string]) (*ChatResponse, error) {

			// === COMPLETION ===
			resp, err := genkit.Generate(ctx, agent.genKitInstance,
				ai.WithModelName("openai/"+agent.ModelID),
				ai.WithSystem(agent.SystemInstructions),
				ai.WithPrompt(input.UserMessage),
				ai.WithConfig(agent.Config),
				ai.WithMessages(
					agent.Messages...,
				),
				ai.WithStreaming(func(ctx context.Context, chunk *ai.ModelResponseChunk) error {
					return callback(ctx, chunk.Text())
				}),
			)
			if err != nil {
				return nil, err
			}
			// === CONVERSATIONAL MEMORY ===

			// USER MESSAGE: append user message to history
			agent.Messages = append(agent.Messages, ai.NewUserTextMessage(strings.TrimSpace(input.UserMessage)))
			// ASSISTANT MESSAGE: append assistant response to history
			agent.Messages = append(agent.Messages, ai.NewModelTextMessage(strings.TrimSpace(resp.Text())))

			// DEBUG: print conversation history
			displayConversationHistory(agent)

			return &ChatResponse{Response: resp.Text()}, nil
		})
	agent.chatStreamFlow = chatStreamFlow
}

func (agent *LocalAIAgent) AskQuestion(question string) (string, error) {
	return "", nil
}

func (agent *LocalAIAgent) AskQuestionStream(question string, callback func(string) error) (string, error) {
	if agent.chatStreamFlow == nil {
		return "", fmt.Errorf("chat stream flow is not initialized")
	}
	// Streaming channel of results
	streamCh := agent.chatStreamFlow.Stream(agent.ctx, &ChatRequest{
		UserMessage: question,
	})

	finalAnswer := ""
	for result := range streamCh {

		if !result.Done {
			finalAnswer += result.Stream
			err := callback(result.Stream)
			if err != nil {
				return "", err
			}
		}
	}

	return finalAnswer, nil
}
