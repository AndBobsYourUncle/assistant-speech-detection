package speech_extraction

import (
	"assistant-speech-detection/ring_buffer"
	"assistant-speech-detection/speech_extraction/vad"
	"assistant-speech-detection/speech_to_text"
	"fmt"
	"github.com/go-audio/audio"
	"github.com/gordonklaus/portaudio"
	"log"
	"time"
)

const (
	quietTimePeriod = time.Millisecond * 200
	bufferSize      = 8196
)

type voiceImpl struct {
	audioRunning bool
	sttEngine    speech_to_text.Interface
}

type Config struct {
	STTEngine speech_to_text.Interface
}

func New(cfg *Config) (Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.STTEngine == nil {
		return nil, fmt.Errorf("sttEngine is nil")
	}

	return &voiceImpl{
		sttEngine: cfg.STTEngine,
	}, nil
}

func (v *voiceImpl) Listen() error {
	err := v.initAudio()
	if err != nil {
		return err
	}

	defer v.freeAudio()

	err = v.listenLoop()
	if err != nil {
		return err
	}

	return nil
}

func (v *voiceImpl) listenLoop() error {
	for {
		waveFilename, err := v.listenIntoBuffer(quietTimePeriod)
		if err != nil {
			log.Fatalf("error listening: %v", err)
		}

		segments, err := v.sttEngine.Process(waveFilename)
		if err != nil {
			log.Printf("error running model: %v", err)

			return err
		}

		for _, segment := range segments {
			log.Printf("[%6s->%6s] %s\n",
				segment.Start.Truncate(time.Millisecond), segment.End.Truncate(time.Millisecond), segment.Text)
		}
	}
}

func (v *voiceImpl) initAudio() error {
	if !v.audioRunning {
		err := portaudio.Initialize()
		if err != nil {
			log.Printf("error initializing audio: %v", err)

			return err
		}

		v.audioRunning = true
	}

	return nil
}

func (v *voiceImpl) freeAudio() {
	if v.audioRunning {
		err := portaudio.Terminate()
		if err != nil {
			log.Printf("Error while freeing audio: %v", err)
		}
	}
}

func (v *voiceImpl) listenIntoBuffer(quietTime time.Duration) (audio.Buffer, error) {
	in := make([]int16, bufferSize)
	stream, err := portaudio.OpenDefaultStream(1, 0, 16000, len(in), in)
	if err != nil {
		return nil, err
	}

	defer stream.Close()

	err = stream.Start()
	if err != nil {
		return nil, err
	}

	var (
		heardSomething bool
		quiet          bool
		quietStart     time.Time
		lastFlux       float64
	)

	vad := vad.New(len(in))

	ringBuffer := ring_buffer.New(bufferSize)

	intBuffer := make([]int, 0)

	for {
		err = stream.Read()
		if err != nil {
			return nil, err
		}

		// keep a buffer of the first bit of audio before detection
		if !heardSomething {
			ringBuffer.Add(in)
		}

		if heardSomething {
			for _, sample := range in {
				intBuffer = append(intBuffer, int(sample))
			}
		}

		flux := vad.Flux(in)

		if lastFlux == 0 {
			lastFlux = flux
			continue
		}

		if heardSomething {
			if flux*1.75 <= lastFlux {
				if !quiet {
					quietStart = time.Now()
				} else {
					diff := time.Since(quietStart)

					if diff > quietTime {
						break
					}
				}

				quiet = true
			} else {
				quiet = false
				lastFlux = flux
			}
		} else {
			if flux >= lastFlux*1.75 {
				heardSomething = true

				// write the first bit of the buffer to the wav buffer
				for _, sample := range in {
					intBuffer = append(intBuffer, int(sample))
				}
			}

			lastFlux = flux
		}
	}

	err = stream.Stop()
	if err != nil {
		return nil, err
	}

	wavBuffer := &audio.IntBuffer{
		Format: &audio.Format{
			NumChannels: 1,
			SampleRate:  16000,
		},
		Data:           intBuffer,
		SourceBitDepth: 16,
	}

	return wavBuffer, nil
}
