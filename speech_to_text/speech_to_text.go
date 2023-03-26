package speech_to_text

import (
	"fmt"
	"github.com/spf13/afero"
	"io"
	"log"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/wav"
)

type sttImpl struct {
	fileSys afero.Fs
	model   whisper.Model
}

type Config struct {
	FileSys afero.Fs
	Model   whisper.Model
}

func New(cfg *Config) (Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.FileSys == nil {
		return nil, fmt.Errorf("fileSys is nil")
	}

	if cfg.Model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	return &sttImpl{
		fileSys: cfg.FileSys,
		model:   cfg.Model,
	}, nil
}

func (stt *sttImpl) Process(wavFilePath string) error {
	var data []float32

	// Create processing context
	context, err := stt.model.NewContext()
	if err != nil {
		return err
	}

	// Open the file
	fh, err := stt.fileSys.Open(wavFilePath)
	if err != nil {
		return err
	}

	defer fh.Close()

	// Decode the WAV file - load the full buffer
	dec := wav.NewDecoder(fh)
	if buf, bufErr := dec.FullPCMBuffer(); bufErr != nil {
		return bufErr
	} else if dec.SampleRate != whisper.SampleRate {
		return fmt.Errorf("unsupported sample rate: %d", dec.SampleRate)
	} else if dec.NumChans != 1 {
		return fmt.Errorf("unsupported number of channels: %d", dec.NumChans)
	} else {
		data = buf.AsFloat32Buffer().Data
	}

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
