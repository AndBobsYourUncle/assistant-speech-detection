package main

import (
	"assistant-speech-detection/clients/ai_bot"
	"assistant-speech-detection/listener"
	"assistant-speech-detection/speech_to_text"
	"flag"
	"log"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

func main() {
	modelFlag := flag.String("m", "", "model file for whisper")
	deviceFlag := flag.String("d", "", "device id to use")
	aiClientHostFlag := flag.String("a", "", "ai client host")

	flag.Parse()

	if modelFlag == nil || *modelFlag == "" {
		log.Fatalf("error: model file not specified")
	}

	if aiClientHostFlag == nil || *aiClientHostFlag == "" {
		log.Fatalf("error: ai client host not specified")
	}

	deviceIDString := ""

	if deviceFlag != nil {
		deviceIDString = *deviceFlag
	}

	// Load model
	model, err := whisper.New(*modelFlag)
	if err != nil {
		log.Fatalf("error loading model: %v", err)
	}

	defer model.Close()

	sstEngine, err := speech_to_text.New(&speech_to_text.Config{
		Model: model,
	})
	if err != nil {
		log.Fatalf("error with speech_to_text.New: %v", err)
	}

	aiBotClient, err := ai_bot.NewClient(&ai_bot.Config{
		ApiHost: *aiClientHostFlag,
	})
	if err != nil {
		log.Fatalf("error with ai_bot.NewClient: %v", err)
	}

	detect, err := listener.New(&listener.Config{
		DeviceID:    deviceIDString,
		STTEngine:   sstEngine,
		AIBotClient: aiBotClient,
	})
	if err != nil {
		log.Fatalf("error with listener.New: %v", err)
	}

	err = detect.ListenLoop()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
