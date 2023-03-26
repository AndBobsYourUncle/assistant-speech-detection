package speech_to_text

import (
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/audio"
)

type Interface interface {
	Process(wavBuffer audio.Buffer) ([]whisper.Segment, error)
}
