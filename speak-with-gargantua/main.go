package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"speak-with-gargantua/toolbox/pico"
)


func main() {
	//ctx := context.Background()

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("🤖🌎🧠 ask me something - /bye to exit> ")
		userMessage, _ := reader.ReadString('\n')

		if strings.HasPrefix(userMessage, "/bye") {
			fmt.Println("👋 Bye!")
			break
		}

		remoteGargantuaAgent := &pico.RemoteAIAgent{
			Name: "Gargantua",
			Endpoint: "http://0.0.0.0:9106/chat-stream-flow",
		}

		remoteGargantuaAgent.AskQuestionStream(userMessage, func(chunk string) error {
			fmt.Print(chunk)
			return nil
		})

		fmt.Println()
		fmt.Println()
	}

}
