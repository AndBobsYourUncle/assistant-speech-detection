package main

import (
	"assistant-speech-detection/speech_extraction"
	"flag"
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/spf13/afero"
	"log"
)

func main() {
	modelFlag := flag.String("m", "", "model file for whisper")

	flag.Parse()

	if *modelFlag == "" {
		log.Fatalf("error: model file not specified")
	}

	// Load model
	model, err := whisper.New(*modelFlag)
	if err != nil {
		log.Fatalf("error loading model: %v", err)
	}

	defer model.Close()

	tempFS := afero.NewMemMapFs()

	detect := speech_extraction.NewVoice(tempFS, model)

	err = detect.Listen()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}
