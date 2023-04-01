package main

import (
	"assistant-speech-detection/listener"
	"assistant-speech-detection/speech_to_text"
	"flag"
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"log"
)

func main() {
	modelFlag := flag.String("m", "", "model file for whisper")

	flag.Parse()

	if modelFlag == nil || *modelFlag == "" {
		log.Fatalf("error: model file not specified")
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

	detect, err := listener.New(&listener.Config{
		STTEngine: sstEngine,
	})
	if err != nil {
		log.Fatalf("error with listener.New: %v", err)
	}

	err = detect.ListenLoop()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
