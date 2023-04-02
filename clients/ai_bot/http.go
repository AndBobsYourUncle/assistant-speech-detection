package ai_bot

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
)

type clientImpl struct {
	apiHost string
}

type Config struct {
	ApiHost string
}

func NewClient(cfg *Config) (AIBotAPI, error) {
	if cfg == nil {
		return nil, errors.New("missing parameter: cfg")
	}

	if cfg.ApiHost == "" {
		return nil, errors.New("missing parameter: cfg.ApiHost")
	}

	return &clientImpl{
		apiHost: cfg.ApiHost,
	}, nil
}

func (client *clientImpl) SendPrompt(ctx context.Context, prompt string) (string, error) {
	req, err := http.NewRequest("GET", client.apiHost+"/get_prompt_response", nil)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	q := req.URL.Query()
	q.Add("prompt", prompt)
	req.URL.RawQuery = q.Encode()

	// Send req using http Client
	httpClient := &http.Client{}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	// get the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
