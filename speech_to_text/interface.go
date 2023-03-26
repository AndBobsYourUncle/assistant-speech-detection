package speech_to_text

import "github.com/go-audio/audio"

type Interface interface {
	Process(wavBuffer audio.Buffer) error
}
