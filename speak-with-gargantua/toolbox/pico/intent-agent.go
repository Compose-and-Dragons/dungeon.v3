package pico

import (
	"context"
	"speak-with-gargantua/toolbox/conversion"
	"speak-with-gargantua/toolbox/env"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Intent struct {
	Intent string `json:"intent"`
}

type IntentAIAgent struct {
	ctx                context.Context
	Name               string
	SystemInstructions string
	ModelID            string
	Messages           []*ai.Message

	Config *openai.ChatCompletionNewParams

	genKitInstance *genkit.Genkit
	intentFlow *core.Flow[*ChatRequest, Intent, struct{}]
}

func (agent *IntentAIAgent) GetName() string {
	return agent.Name
}

func (agent *IntentAIAgent) GetMessages() []*ai.Message {
	return agent.Messages
}

func NewIntentAIAgent(ctx context.Context, name, systemInstructions, modelID, engineURL string, config *openai.ChatCompletionNewParams) *IntentAIAgent {
	oaiPlugin := &oai.OpenAI{
		APIKey: "I💙DockerModelRunner",
		Opts: []option.RequestOption{
			option.WithBaseURL(engineURL),
		},
	}

	genKitInstance := genkit.Init(ctx, genkit.WithPlugins(oaiPlugin))

	agent := &IntentAIAgent{
		Name:               name,
		SystemInstructions: systemInstructions,
		ModelID:            modelID,
		Messages:           []*ai.Message{},
		Config:             config,

		ctx:            ctx,
		genKitInstance: genKitInstance,
	}

	initializeIntentFlow(agent)

	return agent

}

func initializeIntentFlow(agent *IntentAIAgent) {
	intentFlow := genkit.DefineFlow(agent.genKitInstance, "routing-flow",
		func(ctx context.Context, input *ChatRequest) (Intent, error) {
			// Step 1: Determine intent (which NPC to speak to)
			intent, modelResponse, err := genkit.GenerateData[Intent](ctx, agent.genKitInstance,
				ai.WithModelName("openai/"+agent.ModelID),
				ai.WithSystem(agent.SystemInstructions),
				ai.WithPrompt(input.UserMessage),
				ai.WithConfig(agent.Config),
			)
			if err != nil {

				return Intent{}, err
			}
			displayIntentResult(intent, modelResponse)

			return *intent, nil
		})

	agent.intentFlow = intentFlow
}

func displayIntentResult(intent *Intent, modelResponse *ai.ModelResponse) {
	shouldIDisplay := env.GetEnvOrDefault("LOG_MESSAGES", "false")
	
	if conversion.StringToBool(shouldIDisplay) {
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println("🧝 Intent:", intent)
		fmt.Println("📝 Raw model response:", modelResponse.Text())
		fmt.Println(strings.Repeat("-", 50))
	}
}

func (agent *IntentAIAgent) DetectIntent(message string) (Intent, error) {
	return agent.intentFlow.Run(agent.ctx, &ChatRequest{
		UserMessage: message,
	})
}

func (agent *IntentAIAgent) DisplayConversationHistory() {
	// For debugging: print conversation history
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
