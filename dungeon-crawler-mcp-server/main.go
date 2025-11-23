package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dungeon-mcp-server/data"
	"dungeon-mcp-server/tools"
	"dungeon-mcp-server/types"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/mark3labs/mcp-go/server"
	"github.com/openai/openai-go/option"

	"dungeon-mcp-server/conversion"
	"dungeon-mcp-server/env"
)

func main() {

	// ---------------------------------------------------------
	// NOTE: Create [MCP Server]
	// ---------------------------------------------------------
	s := server.NewMCPServer(
		"dungeon-mcp-server",
		"0.0.0",
	)

	// ---------------------------------------------------------
	// Create a "micro" agent
	// ---------------------------------------------------------
	ctx := context.Background()

	fmt.Println("🤖 Initializing Dungeon Agent...")
	baseURL := env.GetEnvOrDefault("MODEL_RUNNER_BASE_URL", "http://localhost:12434/engines/v1/")
	fmt.Println("🌍 Model Runner Base URL:", baseURL)
	dungeonModel := env.GetEnvOrDefault("DUNGEON_MODEL", "ai/qwen2.5:1.5B-F16")
	fmt.Println("🧠 Dungeon Model:", dungeonModel)

	temperature := conversion.StringToFloat(env.GetEnvOrDefault("DUNGEON_MODEL_TEMPERATURE", "0.7"))

	//dungeonAgent := agents.NPCAgent{}
	//dungeonAgent.Initialize(ctx, config, "dungeon-agent")

	// ---------------------------------------------------------
	// Game initialisation
	// ---------------------------------------------------------

	// NOTE: Initialize the Player struct
	currentPlayer := types.Player{
		Name: "Unknown",
	}

	width := conversion.StringToInt(env.GetEnvOrDefault("DUNGEON_WIDTH", "3"))
	height := conversion.StringToInt(env.GetEnvOrDefault("DUNGEON_HEIGHT", "3"))
	entranceX := conversion.StringToInt(env.GetEnvOrDefault("DUNGEON_ENTRANCE_X", "0"))
	entranceY := conversion.StringToInt(env.GetEnvOrDefault("DUNGEON_ENTRANCE_Y", "0"))
	exitX := conversion.StringToInt(env.GetEnvOrDefault("DUNGEON_EXIT_X", "3"))
	exitY := conversion.StringToInt(env.GetEnvOrDefault("DUNGEON_EXIT_Y", "3"))

	dungeonName := env.GetEnvOrDefault("DUNGEON_NAME", "The Dark Labyrinth")
	dungeonDescription := env.GetEnvOrDefault("DUNGEON_DESCRIPTION", "A sprawling underground maze filled with monsters, traps, and treasure.")

	fmt.Println("🧙 Dungeon Name:", dungeonName)
	fmt.Println("📝 Dungeon Description:", dungeonDescription)

	fmt.Println("🏰 Dungeon Size:", width, "x", height)

	// NOTE: Initialize the Dungeon structure
	dungeon := types.Dungeon{
		Name:        dungeonName,
		Description: dungeonDescription,
		Width:       width,
		Height:      height,
		Rooms:       []types.Room{},
		EntranceCoords: types.Coordinates{
			X: entranceX,
			Y: entranceY,
		},
		ExitCoords: types.Coordinates{
			X: exitX,
			Y: exitY,
		},
	}

	// make the dungeon settings configurable via env vars or a config file
	fmt.Println("🚪 Dungeon Entrance Coords:", dungeon.EntranceCoords)
	fmt.Println("🚪 Dungeon Exit Coords:", dungeon.ExitCoords)

	oaiPlugin := &openai.OpenAI{
		APIKey: "I💙DockerModelRunner",
		Opts: []option.RequestOption{
			option.WithBaseURL(baseURL),
		},
	}
	genKitInstance := genkit.Init(ctx, genkit.WithPlugins(oaiPlugin))

	dungeonAgentRoomSystemInstruction := env.GetEnvOrDefault("DUNGEON_AGENT_ROOM_SYSTEM_INSTRUCTION", "You are a Dungeon Master. You create rooms in a dungeon. Each room has a name and a short description.")

	roomGenerationFlow := genkit.DefineFlow(genKitInstance, "room-generation-flow",
		func(ctx context.Context, input *types.ChatRequest) (*data.Room, error) {

			nonPlayerCharacter, modelResponse, err := genkit.GenerateData[data.Room](ctx, genKitInstance,
				ai.WithModelName("openai/"+dungeonModel),
				ai.WithSystem(dungeonAgentRoomSystemInstruction),
				ai.WithPrompt(input.Message),
				ai.WithConfig(map[string]any{"temperature": temperature}),
			)
			if err != nil {
				return nil, err
			}
			fmt.Println("Raw model response:", modelResponse.Text())
			return nonPlayerCharacter, nil

		})

	dungeonAgentMonsterSystemInstruction := env.GetEnvOrDefault("DUNGEON_AGENT_MONSTER_SYSTEM_INSTRUCTION", "You are a Dungeon Master. You create monsters for a dungeon. Each monster has a name, description, and stats.")

	monsterGenerationFlow := genkit.DefineFlow(genKitInstance, "monster-generation-flow",
		func(ctx context.Context, input *types.ChatRequest) (*data.Monster, error) {

			monster, modelResponse, err := genkit.GenerateData[data.Monster](ctx, genKitInstance,
				ai.WithModelName("openai/"+dungeonModel),
				ai.WithSystem(dungeonAgentMonsterSystemInstruction),
				ai.WithPrompt(input.Message),
				ai.WithConfig(map[string]any{"temperature": temperature}),
			)
			if err != nil {
				return nil, err
			}
			fmt.Println("Raw model response:", modelResponse.Text())
			return monster, nil

		})

	// ---------------------------------------------------------
	// BEGIN: Generate the entrance room with the dungeon agent
	// ---------------------------------------------------------
	//dungeonAgent.SetSystemInstructions(dungeonAgentRoomSystemInstruction)

	// Run the flow once to test it
	roomResponse, err := roomGenerationFlow.Run(ctx, &types.ChatRequest{
		Message: "Create an dungeon entrance room with a name and a short description.",
	})

	if err != nil {
		fmt.Println("🔴 Error generating room:", err)
		return
	}

	fmt.Println("📝 Dungeon Entrance Room Response:", roomResponse)

	fmt.Println("👋🏰 Entrance Room:", roomResponse)
	// ---------------------------------------------------------
	// END: of Generate the entrance room with the dungeon agent
	// ---------------------------------------------------------
	// NOTE: Initialize the Room structure
	entranceRoom := types.Room{
		ID:          "room_0_0",
		Name:        roomResponse.Name,
		Description: roomResponse.Description,
		IsEntrance:  true,
		IsExit:      false,
		Coordinates: types.Coordinates{
			X: entranceX,
			Y: entranceY,
		},
		Visited:               true,
		HasMonster:            false,
		HasNonPlayerCharacter: false,
		HasTreasure:           false,
		HasMagicPotion:        false,
	}
	dungeon.Rooms = append(dungeon.Rooms, entranceRoom)

	// ---------------------------------------------------------
	// TOOLS Registration
	// ---------------------------------------------------------
	// ---------------------------------------------------------
	// Register tools and their handlers
	// 🤚 These tools will be used by the dungeon-master program
	// ---------------------------------------------------------
	// Create Player
	createPlayerToolInstance := tools.CreatePlayerTool()
	s.AddTool(createPlayerToolInstance, tools.CreatePlayerToolHandler(&currentPlayer, &dungeon))

	// Get Player Info
	getPlayerInfoToolInstance := tools.GetPlayerInformationTool()
	s.AddTool(getPlayerInfoToolInstance, tools.GetPlayerInformationToolHandler(&currentPlayer, &dungeon))

	// Get Dungeon Info
	getDungeonInfoToolInstance := tools.GetDungeonInformationTool()
	s.AddTool(getDungeonInfoToolInstance, tools.GetDungeonInformationToolHandler(&currentPlayer, &dungeon))

	// Move in the dungeon (two variants with same handler)
	moveIntoTheDungeonToolInstance := tools.GetMoveIntoTheDungeonTool()
	s.AddTool(moveIntoTheDungeonToolInstance, tools.MoveByDirectionToolHandler(&currentPlayer, &dungeon, roomGenerationFlow, monsterGenerationFlow))

	movePlayerToolInstance := tools.GetMovePlayerTool()
	s.AddTool(movePlayerToolInstance, tools.MoveByDirectionToolHandler(&currentPlayer, &dungeon, roomGenerationFlow, monsterGenerationFlow))

	// Get Current Room Info
	getCurrentRoomInfoToolInstance := tools.GetCurrentRoomInformationTool()
	s.AddTool(getCurrentRoomInfoToolInstance, tools.GetCurrentRoomInformationToolHandler(&currentPlayer, &dungeon))

	// Get Dungeon Map
	getDungeonMapToolInstance := tools.GetDungeonMapTool()
	s.AddTool(getDungeonMapToolInstance, tools.GetDungeonMapToolHandler(&currentPlayer, &dungeon))

	// Collect Gold
	collectGoldToolInstance := tools.CollectGoldTool()
	s.AddTool(collectGoldToolInstance, tools.CollectGoldToolHandler(&currentPlayer, &dungeon))

	// Collect Magic Potion
	collectMagicPotionToolInstance := tools.CollectMagicPotionTool()
	s.AddTool(collectMagicPotionToolInstance, tools.CollectMagicPotionToolHandler(&currentPlayer, &dungeon))

	// Fight Monster
	fightMonsterToolInstance := tools.FightMonsterTool()
	s.AddTool(fightMonsterToolInstance, tools.FightMonsterToolHandler(&currentPlayer, &dungeon))

	// Check if Player is in the same room as an NPC
	isPlayerInSameRoomAsNPCToolInstance := tools.IsPlayerInSameRoomAsNPCTool()
	s.AddTool(isPlayerInSameRoomAsNPCToolInstance, tools.IsPlayerInSameRoomAsNPCToolHandler(&currentPlayer, &dungeon))

	// ---------------------------------------------------------
	// NOTE: Start the [Streamable HTTP MCP server]
	// ---------------------------------------------------------
	httpPort := env.GetEnvOrDefault("MCP_HTTP_PORT", "9090")
	fmt.Println("🌍 MCP HTTP Port:", httpPort)

	log.Println("[Dungeon]MCP StreamableHTTP server is running on port", httpPort)

	// Create a custom mux to handle both MCP and health endpoints
	mux := http.NewServeMux()
	// Add healthcheck endpoint (for Docker MCP Gateway with Docker Compose)
	mux.HandleFunc("/health", healthCheckHandler)
	// Add MCP endpoint
	httpServer := server.NewStreamableHTTPServer(s,
		server.WithEndpointPath("/mcp"),
	)
	// Register MCP handler with the mux
	mux.Handle("/mcp", httpServer)

	// Create HTTP server
	srv := &http.Server{
		Addr:    ":" + httpPort,
		Handler: mux,
	}

	// Start the HTTP server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Setup signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for interrupt signal
	<-quit
	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]any{
		"status": "healthy",
	}
	json.NewEncoder(w).Encode(response)
}
