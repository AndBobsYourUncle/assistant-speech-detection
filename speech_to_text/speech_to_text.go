package speech_to_text

import (
	"fmt"
	"github.com/go-audio/audio"
	"io"
	"log"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type sttImpl struct {
	model whisper.Model
}

type Config struct {
	Model whisper.Model
}

func New(cfg *Config) (Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.Model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	return &sttImpl{
		model: cfg.Model,
	}, nil
}

func (stt *sttImpl) Process(wavBuffer audio.Buffer) error {
	// Create processing context
	context, err := stt.model.NewContext()
	if err != nil {
		return err
	}

	data := wavBuffer.AsFloat32Buffer().Data

	// Segment callback when -tokens is specified
	var cb whisper.SegmentCallback

	err = context.Process(data, cb)
	if err != nil {
		return err
	}

	// Print out the results
	return outputSegments(context)
}

// Output text to terminal
func outputSegments(context whisper.Context) error {
	seenText := make(map[string]bool)

	for {
		segment, err := context.NextSegment()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		// if segment text starts or ends with a parenthesis or a bracket, then ignore it
		if len(segment.Text) > 0 && (segment.Text[0] == '(' || segment.Text[0] == '[' ||
			segment.Text[len(segment.Text)-1] == ')' || segment.Text[len(segment.Text)-1] == ']') {
			continue
		}

		// if we've already seen this text, then ignore it
		if _, ok := seenText[segment.Text]; ok {
			continue
		} else {
			seenText[segment.Text] = true
		}

		log.Printf("[%6s->%6s] %s\n",
			segment.Start.Truncate(time.Millisecond), segment.End.Truncate(time.Millisecond), segment.Text)
	}
}
