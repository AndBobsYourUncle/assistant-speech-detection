package speech_to_text

import (
	"fmt"
	"io"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/audio"
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

func (stt *sttImpl) Process(wavBuffer audio.Buffer) ([]whisper.Segment, error) {
	// Create processing context
	context, err := stt.model.NewContext()
	if err != nil {
		return nil, err
	}

	data := wavBuffer.AsFloat32Buffer().Data

	// Segment callback when -tokens is specified
	var cb whisper.SegmentCallback

	err = context.Process(data, cb)
	if err != nil {
		return nil, err
	}

	segments, err := outputSegments(context)
	if err != nil {
		return nil, err
	}

	return segments, nil
}

// Output text to terminal
func outputSegments(context whisper.Context) ([]whisper.Segment, error) {
	seenText := make(map[string]bool)

	segments := make([]whisper.Segment, 0)

	for {
		segment, err := context.NextSegment()
		if err == io.EOF {
			return segments, nil
		} else if err != nil {
			return nil, err
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

		segments = append(segments, segment)
	}
}
