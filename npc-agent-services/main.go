package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/localvec"
	"github.com/firebase/genkit/go/plugins/server"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	// ✋🛠️ toolbox
	"npc-agent-services/toolbox/env"
	"npc-agent-services/toolbox/conversion"
	"npc-agent-services/toolbox/files"
	"npc-agent-services/toolbox/rag"
)

// Structure for flow input
type ChatRequest struct {
	UserMessage string `json:"message"`
}

// Structure for final flow output
type ChatResponse struct {
	Response string `json:"response"`
}

func main() {
	ctx := context.Background()
	// === SETTINGS ===
	engineURL := env.GetEnvOrDefault("ENGINE_BASE_URL", "http://localhost:12434/engines/v1/")
	modelID := env.GetEnvOrDefault("CHAT_MODEL", "ai/qwen2.5:1.5B-F16")
	embeddingModelID := env.GetEnvOrDefault("EMBEDDING_MODEL", "ai/mxbai-embed-large:latest")

	storeName := env.GetEnvOrDefault("STORE_NAME", "npc-agent-store")

	// agentName := env.GetEnvOrDefault("AGENT_NAME", "Unknown")
	// agentRace := env.GetEnvOrDefault("AGENT_RACE", "Unknown")
	// fmt.Println("🤖 Initializing NPC Agent:", agentName, "the", agentRace)

	temperature := conversion.StringToFloat(env.GetEnvOrDefault("AGENT_MODEL_TEMPERATURE", "0.7"))
	topP := conversion.StringToFloat(env.GetEnvOrDefault("AGENT_MODEL_TOP_P", "0.9"))

	systemInstructionsPath := env.GetEnvOrDefault("SYSTEM_INSTRUCTIONS_PATH", "./data/sorcerer_system_instructions.md")
	systemInstructions, err := files.ReadTextFile(systemInstructionsPath)
	if err != nil {
		log.Fatal(err)
	}

	backgroundPath := env.GetEnvOrDefault("BACKGROUND_CONTEXT_PATH", "./data/sorcerer_background_and_personality.md")
	backgroundContext, err := files.ReadTextFile(backgroundPath)
	if err != nil {
		log.Fatal(err)
	}

	oaiPlugin := &oai.OpenAI{
		APIKey: "I💙DockerModelRunner",
		Opts: []option.RequestOption{
			option.WithBaseURL(engineURL),
		},
	}

	genKitInstance := genkit.Init(ctx, genkit.WithPlugins(oaiPlugin))

	// === RAG ===
	if err := localvec.Init(); err != nil {
		log.Fatal(err)
	}
	// STEP 1: Embedder definition/creation
	// you don't need to prefix the model name with the provider
	embedder := oaiPlugin.DefineEmbedder(embeddingModelID, nil)
	docStore, documentRetriever, err := localvec.DefineRetriever(
		genKitInstance,
		storeName,
		localvec.Config{
			Embedder: embedder,
			Dir:      "./store.db",
		},
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("🔎 Retriever:", documentRetriever)
	fmt.Println("📚 DocStore:", len(docStore.Data))
	// STEP 2: Indexing documents if the store is empty
	if len(docStore.Data) == 0 {
		fmt.Println("🚧 The document store is empty. Proceeding to index documents...")
		// CHUNKS: Split the background context into smaller chunks
		chunks := rag.SplitMarkdownBySections(backgroundContext)
		docs := []*ai.Document{}
		for idx, chunk := range chunks {
			fmt.Println("-", idx, chunk)
			docs = append(docs, ai.DocumentFromText(chunk, nil))
		}
		fmt.Println("🗂️ Indexing documents...", docs)
		err := localvec.Index(ctx, docs, docStore)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("✅ Document indexing completed.")
	} else {
		fmt.Println("✅ The document store is already populated.")
	}

	messages := []*ai.Message{}

	chatStreamFlow := genkit.DefineStreamingFlow(genKitInstance, "chat-stream-flow",
		func(ctx context.Context, input *ChatRequest, callback core.StreamCallback[string]) (*ChatResponse, error) {

			// === SIMILARITY SEARCH ===
			// Create a query document from the user question
			queryDoc := ai.DocumentFromText(input.UserMessage, nil)
			// Create a retriever request with custom options
			request := &ai.RetrieverRequest{
				Query: queryDoc,
			}
			// Retrieve documents relevant to a query
			retrieveResponse, err := documentRetriever.Retrieve(ctx, request)
			if err != nil {
				retrieveResponse = &ai.RetrieverResponse{Documents: []*ai.Document{}}
				log.Println(err)
				//log.Fatal(err)
			}
			//fmt.Println("Retrieved documents:", retrieveResponse.Documents)

			// Process the retrieved documents
			similarDocuments := ""
			for _, doc := range retrieveResponse.Documents {
				//fmt.Println(doc.Metadata, doc.Content[0].Text)
				similarDocuments += doc.Content[0].Text + "\n\n"
			}
			// Inject the similar documents into the conversation as system message
			messages = append(
				messages,
				ai.NewSystemTextMessage(
					fmt.Sprintf("Relevant context to help you answer the next question:\n%s", similarDocuments),
				),
			)

			// === COMPLETION ===
			resp, err := genkit.Generate(ctx, genKitInstance,
				ai.WithModelName("openai/"+modelID),
				ai.WithSystem(systemInstructions),
				ai.WithPrompt(input.UserMessage),
				ai.WithConfig(&openai.ChatCompletionNewParams{
					Temperature: openai.Float(temperature),
					TopP: openai.Float(topP),
				}),
				ai.WithMessages(
					messages...,
				),
				ai.WithStreaming(func(ctx context.Context, chunk *ai.ModelResponseChunk) error {
					return callback(ctx, chunk.Text())
				}),
			)
			if err != nil {
				return nil, err
			}
			// === CONVERSATIONAL MEMORY ===

			// NOTE: remove the last message in messages (similar documents system message) to avoid accumulation
			messages = messages[:len(messages)-1]


			// USER MESSAGE: append user message to history
			messages = append(messages, ai.NewUserTextMessage(strings.TrimSpace(input.UserMessage)))
			// ASSISTANT MESSAGE: append assistant response to history
			messages = append(messages, ai.NewModelTextMessage(strings.TrimSpace(resp.Text())))

			fmt.Println()
			fmt.Println(strings.Repeat("-", 50))
			fmt.Println("🗒️ Conversation history:")
			for _, msg := range messages {
				content := msg.Content[0].Text
				if len(content) > 80 {
					fmt.Println("📝", msg.Role, ":", content[:80]+"...")
				} else {
					fmt.Println("📝", msg.Role, ":", content)
				}
			}
			
			return &ChatResponse{Response: resp.Text()}, nil
		})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat-stream-flow", genkit.Handler(chatStreamFlow))
	log.Println("Starting server on http://localhost:9100")
	log.Println("Flow available at: POST http://localhost:9100/chat-stream-flow")
	log.Fatal(server.Start(ctx, "0.0.0.0:9100", mux))
}
