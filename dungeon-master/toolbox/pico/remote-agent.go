package pico

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/firebase/genkit/go/ai"
)

// Structure for flow input
type RemoteChatRequest struct {
	Data struct {
		Message string `json:"message"`
	} `json:"data"`
}

type RemoteAIAgent struct {
	Endpoint string
	Name     string
}

func (agent *RemoteAIAgent) GetName() string {
	return agent.Name
}

func (agent *RemoteAIAgent) GetMessages() []*ai.Message {
	// TODO: Implement if needed
	return nil
}

func (agent *RemoteAIAgent) AskQuestion(question string) (string, error) {
	return "", nil
}

func (agent *RemoteAIAgent) AskQuestionStream(question string, callback func(string) error) (string, error) {
	// Prepare request
	reqBody := RemoteChatRequest{}
	reqBody.Data.Message = strings.TrimSpace(question)

	// Convert to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("Error when creating JSON: %v\n", err)
		//continue
		return "", err
	}
	// Create HTTP request
	req, err := http.NewRequest("POST", agent.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error when creating the request: %v\n", err)
		//continue
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error when HTTP call: %v\n", err)
		//continue
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("HTTP error: status code %d\n", resp.StatusCode)
		resp.Body.Close()
		//continue
		return "", fmt.Errorf("HTTP error: status code %d", resp.StatusCode)
	}

	// Read the stream
	streamReader := bufio.NewReader(resp.Body)
	fullResponse := ""
	var callbackErr error
	for {
		line, err := streamReader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			callbackErr = err
			fmt.Printf("\nError when stream reading: %v\n", err)
			break
		}

		// Read data lines starting with "data: "
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			data := after

			// Check for end of stream
			if data == "[DONE]" {
				break
			}

			// Parse JSON to extract content
			var chunk map[string]any
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				// Display content if available (try "message" then "text")
				if message, ok := chunk["message"].(string); ok {
					//fmt.Print(message)
					fullResponse += message
					callback(message)
				} else if text, ok := chunk["text"].(string); ok {
					//fmt.Print(text)
					fullResponse += text
					callback(text)
				}
			} else {
				// If not JSON, print as is
				//fmt.Print(data)
				fullResponse += data
				callback(data)
			}
			callbackErr = err
		}
	}
	resp.Body.Close()
	return fullResponse, callbackErr
}
