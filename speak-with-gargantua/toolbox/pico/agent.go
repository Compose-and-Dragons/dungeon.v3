package pico

import (
	"github.com/firebase/genkit/go/ai"
)

type AIAgent interface {
	AskQuestion(question string) (string, error)
	AskQuestionStream(question string, callback func(string) error) (string, error)
	GetName() string
	GetMessages() []*ai.Message
}
