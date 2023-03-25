package speech_to_text

import (
	"fmt"
	"github.com/spf13/afero"
	"io"
	"log"
	"os"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/wav"
)

func Process(fileSys afero.Fs, model whisper.Model, path string) error {
	var data []float32

	// Create processing context
	context, err := model.NewContext()
	if err != nil {
		return err
	}

	// Open the file
	fh, err := fileSys.Open(path)
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
	return Output(os.Stdout, context)
}

// Output text to terminal
func Output(w io.Writer, context whisper.Context) error {
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
