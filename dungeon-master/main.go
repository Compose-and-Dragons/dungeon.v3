package main

import (
	"context"
	"encoding/json"

	"log"
	"strings"

	"github.com/openai/openai-go"

	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/mcp"

	"dungeon-master/toolbox/conversion"
	"dungeon-master/toolbox/env"
	"dungeon-master/toolbox/mcpcatalog"
	"dungeon-master/toolbox/pico"
	"dungeon-master/toolbox/ui"
)

func main() {

	ctx := context.Background()

	engineURL := env.GetEnvOrDefault("MODEL_RUNNER_BASE_URL", "http://localhost:12434/engines/llama.cpp/v1")
	mcpHost := env.GetEnvOrDefault("MCP_SERVER_BASE_URL", "http://localhost:9011/mcp")

	intentModelId := env.GetEnvOrDefault("DUNGEON_MASTER_INTENT_MODEL", "hf.co/menlo/jan-nano-gguf:q4_k_m")
	toolsModelId := env.GetEnvOrDefault("DUNGEON_MASTER_TOOLS_MODEL", "hf.co/menlo/jan-nano-gguf:q4_k_m")
	chatModelId := env.GetEnvOrDefault("DUNGEON_MASTER_CHAT_MODEL", "hf.co/menlo/jan-nano-gguf:q4_k_m")

	dungeonMasterModel := "openai/" + env.GetEnvOrDefault("DUNGEON_MASTER_MODEL", "hf.co/menlo/jan-nano-gguf:q4_k_m")

	fmt.Println("🌍 LLM URL:", engineURL)
	fmt.Println("🌍 MCP Host:", mcpHost)
	fmt.Println("🌍 Dungeon Master Model:", dungeonMasterModel)

	// STEP 2: Create the MCP Client to connect to the MCP Dungeon Server
	// [MCP Client] to connect to the [MCP Dungeon Server]
	mcpClient, err := mcp.NewGenkitMCPClient(mcp.MCPClientOptions{
		Name: "c&d",
		StreamableHTTP: &mcp.StreamableHTTPConfig{
			BaseURL: env.GetEnvOrDefault("MCP_SERVER_BASE_URL", "http://localhost:9011/mcp"), // docker-mcp-gateway
		},
	})
	if err != nil {
		panic(fmt.Errorf("failed to create MCP client: %v", err))
	}

	ui.Println(ui.Orange, "MCP Client initialized successfully")

	// ---------------------------------------------------------
	// Get the [MCP Tools Index] from the [MCP Client]
	// ---------------------------------------------------------
	toolsRefs, err := mcpcatalog.GetToolsList(ctx, mcpClient)
	if err != nil {
		log.Fatal("😡 Error getting MCP Tools Catalog:", err)
	}
	ui.Println(ui.Orange, "MCP Tools Catalog retrieved successfully.")
	DisplayToolsCatalog(toolsRefs)

	// ---------------------------------------------------------
	// AGENTS:
	// Agent using tools
	// Agent using intent
	// ---------------------------------------------------------
	dungeonMasterAgentName := env.GetEnvOrDefault("DUNGEON_MASTER_NAME", "Zephyr")
	dungeonMasterModeltemperature := conversion.StringToFloat(env.GetEnvOrDefault("DUNGEON_MASTER_MODEL_TEMPERATURE", "0.0"))
	dungeonMasterModeltopP := conversion.StringToFloat(env.GetEnvOrDefault("DUNGEON_MASTER_MODEL_TOP_P", "0.9"))

	// INTENT AGENT:
	// zephyrIntentAgent :=
	zephyrIntentAgent := pico.NewIntentAIAgent(
		ctx,
		dungeonMasterAgentName+"Intent",
		`
		You are helping the dungeon master of a D&D game.
		Detect if the user want to speak to one of the following NPCs: 
		Thrain (dwarf blacksmith), Liora (elven mage), Galdor (human rogue), Elara (halfling ranger), Shesepankh (tiefling warlock).

		If the user's message does not explicitly mention wanting to speak to one of these NPCs, respond with "Nobody".
		Otherwise, respond with ONLY the NPC name: Thrain, Liora, Galdor, Elara, Shesepankh.
		`,
		intentModelId,
		engineURL,
		&openai.ChatCompletionNewParams{
			Temperature: openai.Float(0.0),
			//TopP:        openai.Float(dungeonMasterModeltopP),
		},
	)

	// TOOLS AGENT:

	// SYSTEM MESSAGE:
	instructions := fmt.Sprintf(`Your name is "%s the Dungeon Master".`, dungeonMasterAgentName) + "\n" +
		env.GetEnvOrDefault(
			"DUNGEON_MASTER_SYSTEM_INSTRUCTIONS",
			`
			You are a helpful D&D assistant that can roll dice and generate character names.
			Use the appropriate tools when asked to roll dice or generate character names.
			`,
		)

	zephyrToolsAgent := pico.NewToolsAIAgent(
		ctx,
		dungeonMasterAgentName,
		instructions,
		toolsModelId,
		engineURL,
		&openai.ChatCompletionNewParams{
			Temperature: openai.Float(dungeonMasterModeltemperature),
			TopP:        openai.Float(dungeonMasterModeltopP),
		},
		toolsRefs,
	)

	zephyrChatAgent := pico.NewLocalAIAgent(
		ctx,
		dungeonMasterAgentName+"Chat",
		instructions,
		chatModelId,
		engineURL,
		&openai.ChatCompletionNewParams{
			Temperature: openai.Float(0.5),
			TopP:        openai.Float(dungeonMasterModeltopP),
		},
	)

	// ---------------------------------------------------------
	// TEAM: Assemble the agents into a team
	// ---------------------------------------------------------

	// STEP 1: Define the agents team with remote AI agents
	agentsTeam := map[string]pico.AIAgent{
		"zephyr": zephyrToolsAgent,
		"elara": &pico.RemoteAIAgent{
			Name:     "Elara",
			Endpoint: env.GetEnvOrDefault("SORCERER_ENDPOINT", "http://0.0.0.0:9101/chat-stream-flow"),
		},
		"galdor": &pico.RemoteAIAgent{
			Name:     "Galdor",
			Endpoint: env.GetEnvOrDefault("MERCHANT_ENDPOINT", "http://0.0.0.0:9102/chat-stream-flow"),
		},
		"thrain": &pico.RemoteAIAgent{
			Name:     "Thrain",
			Endpoint: env.GetEnvOrDefault("GUARD_ENDPOINT", "http://0.0.0.0:9103/chat-stream-flow"),
		},
		"liora": &pico.RemoteAIAgent{
			Name:     "Liora",
			Endpoint: env.GetEnvOrDefault("HEALER_ENDPOINT", "http://0.0.0.0:9104/chat-stream-flow"),
		},
		"shesepankh": &pico.RemoteAIAgent{
			Name:     "Shesepankh",
			Endpoint: env.GetEnvOrDefault("BOSS_ENDPOINT", "http://0.0.0.0:9105/chat-stream-flow"),
		},
	}
	//fmt.Println(agentsTeam)

	dungeonMaster := strings.ToLower(zephyrToolsAgent.GetName())
	sorcerer := strings.ToLower(agentsTeam["elara"].GetName())
	merchant := strings.ToLower(agentsTeam["galdor"].GetName())
	guard := strings.ToLower(agentsTeam["thrain"].GetName())
	healer := strings.ToLower(agentsTeam["liora"].GetName())
	boss := strings.ToLower(agentsTeam["shesepankh"].GetName())

	selectedAgent := agentsTeam["zephyr"] // Start with the Dungeon Master

	DisplayAgentsTeam(agentsTeam)

	// Loop to interact with the agents
	for {

		var promptText string
		if selectedAgent.GetName() == agentsTeam["zephyr"].GetName() {
			// Dungeon Master prompt
			promptText = "🤖 (/bye to exit) [" + selectedAgent.GetName() + "]>"
		} else {
			// Non Player Character prompt
			promptText = "🙂 (/bye to exit /dm to go back to the DM) [" + selectedAgent.GetName() + "]>"
		}

		// USER PROMPT: (input)
		content, _ := ui.SimplePrompt(promptText, "Type your message here...")

		// ---------------------------------------------------------
		// [COMMAND]: `/bye` command to exit the loop
		// ---------------------------------------------------------
		if strings.HasPrefix(content.Input, "/bye") {
			fmt.Println("👋 Goodbye! Thanks for playing!")
			break
		}

		// ---------------------------------------------------------
		// [COMMAND] `/dm` Back to the Dungeon Master
		// ---------------------------------------------------------
		if strings.HasPrefix(content.Input, "/back-to-dm") || strings.HasPrefix(content.Input, "/dm") || strings.HasPrefix(content.Input, "/dungeonmaster") && selectedAgent.GetName() != dungeonMasterAgentName {
			selectedAgent = agentsTeam["zephyr"]
			ui.Println(ui.Pink, "👋 You are back to the Dungeon Master:", selectedAgent.GetName())
			continue
			/*
				In Go, the continue keyword in a loop immediately jumps to the next iteration of the loop, skipping the rest
				of the code in the current iteration.

				Specifically:
				- In a for loop, continue returns to the beginning of the loop for the next iteration
				- Code after continue in the same iteration is not executed
				- The loop condition is evaluated normally
			*/
		}

		// ---------------------------------------------------------
		// [COMMAND] `/agents` Get the AGENTS team list
		// ---------------------------------------------------------
		if strings.HasPrefix(content.Input, "/agents") {
			DisplayAgentsTeam(agentsTeam)
			continue
		}

		// ---------------------------------------------------------
		// [COMMAND] `/tools` Get the TOOLS list
		// ---------------------------------------------------------
		if strings.HasPrefix(content.Input, "/tools") {
			DisplayToolsCatalog(toolsRefs)
			continue
		}

		switch strings.ToLower(selectedAgent.GetName()) {
		// ---------------------------------------------------------
		//  AGENT: **Dungeon Master** [COMPLETION] with [TOOLS]
		// ---------------------------------------------------------
		case dungeonMaster: // Zephyr the Dungeon Master

			ui.Println(ui.Yellow, "<", selectedAgent.GetName(), "speaking...>")

			// IMPORTANT: TODO: test if the player is created

			// INTENT DETECTION: Switch to the appropriate agent based on intent
			intentResult, err := zephyrIntentAgent.DetectIntent(content.Input)
			if err == nil { // BEGIN:
				selectedNpc, exists := agentsTeam[strings.ToLower(intentResult.Intent)]
				fmt.Println("🧭 Detected intent:", intentResult.Intent, "-> exists in agents team?", exists)

				if exists == true {
					if strings.ToLower(selectedNpc.GetName()) == dungeonMaster {
						// Already speaking to the selected NPC
						ui.Println(ui.Yellow, "🤖 You are already speaking to:", selectedAgent.GetName())
						continue
					} else {

						// ---------------------------------------------------------
						// [Check if you are in the same room as the NPC] [DIRECT CALL TO MCP]
						// ---------------------------------------------------------
						res, _ := zephyrToolsAgent.DirectExecuteTool(ai.ToolRequest{
							Name: "c&d_is_player_in_same_room_as_npc",
							Input: map[string]any{
								"name": strings.ToLower(intentResult.Intent),
							},
							Ref: "",
						})
						inTheSameRoom := IsPlayerInSameRoomAsNPC(res)

						if inTheSameRoom == false {
							ui.Println(ui.Red, "❌ You cannot speak to", selectedNpc.GetName(), "because you are not in the same room!")
							continue
						}

						selectedAgent = selectedNpc
						ui.Printf(ui.Pink, "👋 You are now speaking to %s.\n", selectedAgent.GetName())
						continue
					}

				} else {
					// NPC not found, stay with the current agent
					ui.Println(ui.Yellow, "🤖 Staying with the current agent:", selectedAgent.GetName())
				}

				if strings.ToLower(selectedAgent.GetName()) == dungeonMaster {

					// [TOOL CALLS] DETECTION AND EXECUTION
					toolsResponse, err := zephyrToolsAgent.RunToolCalls(content.Input)

					if err != nil {
						ui.Println(ui.Red, "Error:", err)
						continue
					}

					// DISPLAY TOOL CALL RESULTS
					fmt.Println(strings.Repeat("=", 16) + "[TOOL CALL RESULT]" + strings.Repeat("=", 16))
					fmt.Println(toolsResponse.Text)
					fmt.Println(strings.Repeat("=", 50))

					// STREAMING FINAL RESPONSE TO USER
					// Use the zephyrChatAgent to stream the final response
					_, chatErr := zephyrChatAgent.AskQuestionStream(toolsResponse.Text, func(chunk string) error {
						fmt.Print(chunk)
						return nil
					})

					if chatErr != nil {
						ui.Println(ui.Red, "Error:", err)
					}
					// TODO: Clear history after each interaction to avoid tool call accumulation
				}

			} // END: intent detection

		// ---------------------------------------------------------
		// TALK TO: AGENT:: **GUARD** + [RAG]
		// ---------------------------------------------------------
		case guard:

			ui.Println(ui.Brown, "<", selectedAgent.GetName(), "speaking...>")

			_, chatErr := selectedAgent.AskQuestionStream(content.Input, func(chunk string) error {
				fmt.Print(chunk)
				return nil
			})

			if chatErr != nil {
				ui.Println(ui.Red, "Error:", err)
			}

		// ---------------------------------------------------------
		// TALK TO: AGENT:: **SORCERER** + [RAG]
		// ---------------------------------------------------------
		case sorcerer:

			ui.Println(ui.Purple, "<", selectedAgent.GetName(), "speaking...>")

			_, chatErr := selectedAgent.AskQuestionStream(content.Input, func(chunk string) error {
				fmt.Print(chunk)
				return nil
			})

			if chatErr != nil {
				ui.Println(ui.Red, "Error:", err)
			}

		// ---------------------------------------------------------
		// TALK TO: AGENT:: **HEALER** + [RAG]
		// ---------------------------------------------------------
		case healer:

			ui.Println(ui.Magenta, "<", selectedAgent.GetName(), "speaking...>")

			_, chatErr := selectedAgent.AskQuestionStream(content.Input, func(chunk string) error {
				fmt.Print(chunk)
				return nil
			})

			if chatErr != nil {
				ui.Println(ui.Red, "Error:", err)
			}

		// ---------------------------------------------------------
		// TALK TO: AGENT:: **MERCHANT** + [RAG]
		// ---------------------------------------------------------
		case merchant:

			ui.Println(ui.Cyan, "<", selectedAgent.GetName(), "speaking...>")

			_, chatErr := selectedAgent.AskQuestionStream(content.Input, func(chunk string) error {
				fmt.Print(chunk)
				return nil
			})

			if chatErr != nil {
				ui.Println(ui.Red, "Error:", err)
			}

		// ---------------------------------------------------------
		// TALK TO: AGENT:: **BOSS**
		// ---------------------------------------------------------
		case boss:

			ui.Println(ui.Red, "<", selectedAgent.GetName(), "speaking...>")

			answer, chatErr := selectedAgent.AskQuestionStream(content.Input, func(chunk string) error {
				fmt.Print(chunk)
				return nil
			})

			if chatErr != nil {
				ui.Println(ui.Red, "Error:", err)
			} else {
				// IMPORTANT: Check if the player has defeated the boss
				// ---------------------------------------------------------
				// You lose 😢
				// ---------------------------------------------------------
				if strings.Contains(strings.ToLower(answer), "you are trapped") {
					ui.Println(ui.Red, "\n💀 You have been defeated by the Boss! Game Over! 💀")
					ui.Println(ui.Red, "👹 The Boss reigns supreme in the dungeon! 👹")
					ui.Println(ui.Red, "🎲 Better luck next time! 🎲")
					continue
				}

				// ---------------------------------------------------------
				// You win 🎉
				// ---------------------------------------------------------
				if strings.Contains(strings.ToLower(answer), "you are free") {
					ui.Println(ui.Green, "\n💀 You have defeated the Boss! Congratulations, brave adventurer! 💀")
					ui.Println(ui.Green, "👑 You are now the new ruler of the dungeon! 👑")
					ui.Println(ui.Green, "🎉 Thanks for playing! 🎉")
					continue
				}
			}

		default:
			ui.Printf(ui.Cyan, "\n🤖 %s is thinking...\n", selectedAgent.GetName())

		}
		fmt.Println()
		fmt.Println()

	}

}

// ---------------------------------------------------------
// HELPERS:
// ---------------------------------------------------------

func DisplayAgentsTeam(agentsTeam map[string]pico.AIAgent) {
	for agentId, agent := range agentsTeam {
		ui.Printf(ui.Cyan, "Agent ID: %s agent name: %s\n", agentId, agent.GetName())
	}
	fmt.Println()
}

func DisplayToolsCatalog(tools []ai.ToolRef) {
	ui.Println(ui.Green, "📦 Available Tools:")
	for _, tool := range tools {
		fmt.Println("   -", tool.Name())
	}
	fmt.Println()
}

func IsPlayerInSameRoomAsNPC(toolExecResult string) bool {

	type RoomCheckResult struct {
		InSameRoom   bool   `json:"in_same_room"`
		PlayerRoomID string `json:"player_room_id"`
		Message      string `json:"message"`
	}

	type MCPResponse struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
	}

	var mcpResp MCPResponse
	err := json.Unmarshal([]byte(toolExecResult), &mcpResp)
	if err != nil {
		ui.Println(ui.Red, "❌ Error parsing MCP response:", err)
	}

	var roomCheck RoomCheckResult
	if len(mcpResp.Content) > 0 {
		json.Unmarshal([]byte(mcpResp.Content[0].Text), &roomCheck)
	}

	ui.Println(ui.Green, "❇️ MCP Tool Response Message:", mcpResp)
	ui.Println(ui.Blue, "Ⓜ️ Information Message (Room Check):", roomCheck)

	return roomCheck.InSameRoom

}

func DisplayResultsOfToolCalls(toolsResponse pico.ToolCallsResult) {
	shouldIDisplay := env.GetEnvOrDefault("LOG_MESSAGES", "false")

	if conversion.StringToBool(shouldIDisplay) {
		fmt.Println(strings.Repeat("=", 50))
		fmt.Println("Final Response:", toolsResponse.Text)
		fmt.Println(strings.Repeat("=", 50))

		for index, result := range toolsResponse.List {
			fmt.Printf("%v- %v\n", index, result)
		}
		fmt.Println(strings.Repeat("=", 50))
	}
}
