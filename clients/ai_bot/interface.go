package ai_bot

import "context"

type AIBotAPI interface {
	SendPrompt(ctx context.Context, prompt string) (string, error)
}
