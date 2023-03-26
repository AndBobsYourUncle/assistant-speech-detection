package main

import (
	"assistant-speech-detection/speech_extraction"
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

	detect, err := speech_extraction.New(&speech_extraction.Config{
		STTEngine: sstEngine,
	})
	if err != nil {
		log.Fatalf("error with speech_extraction.New: %v", err)
	}

	err = detect.Listen()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
