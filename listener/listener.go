package listener

import (
	"assistant-speech-detection/listener/vad"
	"assistant-speech-detection/ring_buffer"
	"assistant-speech-detection/speech_to_text"
	"fmt"
	"github.com/go-audio/audio"
	"github.com/gordonklaus/portaudio"
	"log"
	"strings"
	"time"
)

const (
	quietTimePeriod = time.Millisecond * 200
	bufferSize      = 8196
)

type ListenAction string

const (
	ListenActionWait    ListenAction = "wait"
	ListenActionWake    ListenAction = "wake"
	ListenActionCommand ListenAction = "command"
)

type voiceImpl struct {
	audioRunning    bool
	sttEngine       speech_to_text.Interface
	triggeredAction ListenAction
	interrupt       bool
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
		sttEngine:       cfg.STTEngine,
		triggeredAction: ListenActionWake,
	}, nil
}

func (v *voiceImpl) ListenLoop() error {
	err := v.initAudio()
	if err != nil {
		return err
	}

	defer v.freeAudio()

	log.Printf("starting to listen\n")

	for {
		if v.triggeredAction == ListenActionWake {
			err = v.listenForWakeLoop()
			if err != nil {
				return err
			}
		} else if v.triggeredAction == ListenActionCommand {
			err = v.listenLoop()
			if err != nil {
				return err
			}
		}

		time.Sleep(time.Millisecond * 100)
	}
}

func (v *voiceImpl) HaltListening() {
	v.interrupt = true
	v.triggeredAction = ListenActionWait

	log.Printf("waiting due to interrupt\n")
}

func (v *voiceImpl) ListenForWake() {
	v.interrupt = true
	v.triggeredAction = ListenActionWake

	log.Printf("resetting to waiting for wake\n")
}

func (v *voiceImpl) ListenForCommand() {
	v.interrupt = true
	v.triggeredAction = ListenActionWait

	log.Printf("resetting to expecting a command\n")
}

func (v *voiceImpl) listenLoop() error {
	for {
		log.Printf("speak now\n")

		waveBuffer, err := v.listenIntoBuffer(quietTimePeriod, 0)
		if err != nil {
			log.Fatalf("error listening: %v", err)
		}

		segments, err := v.sttEngine.Process(waveBuffer)
		if err != nil {
			log.Printf("error running model: %v", err)

			return err
		}

		for _, segment := range segments {
			log.Printf("[%6s->%6s] %s\n",
				segment.Start.Truncate(time.Millisecond), segment.End.Truncate(time.Millisecond), segment.Text)

			v.triggeredAction = ListenActionWake
			log.Printf("waiting for wake\n")

			return nil
		}
	}
}

func (v *voiceImpl) listenForWakeLoop() error {
	for {
		waveBuffer, err := v.listenIntoBuffer(quietTimePeriod, time.Millisecond*500)
		if err != nil {
			log.Fatalf("error listening: %v", err)
		}

		segments, err := v.sttEngine.Process(waveBuffer)
		if err != nil {
			log.Printf("error running model: %v", err)

			return err
		}

		for _, segment := range segments {
			// extract only alphanumeric characters from the text
			// this is to avoid false positives when the wake word is detected in a sentence
			detectedText := strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ' {
					return r
				}

				return -1
			}, segment.Text)

			if strings.Contains(strings.ToLower(detectedText), "hey smart home") {
				log.Printf("wake word detected: %s\n", segment.Text)

				v.triggeredAction = ListenActionCommand
				log.Printf("expecting a command\n")

				return nil
			}
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

func (v *voiceImpl) listenIntoBuffer(quietTime time.Duration, maxTime time.Duration) (audio.Buffer, error) {
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

	var startTime time.Time

	for {
		if v.interrupt {
			v.interrupt = false

			return &audio.IntBuffer{
				Format: &audio.Format{
					NumChannels: 1,
					SampleRate:  16000,
				},
				Data:           []int{},
				SourceBitDepth: 16,
			}, nil
		}

		err = stream.Read()
		if err != nil {
			return nil, err
		}

		// keep a buffer of the first bit of audio before detection
		if !heardSomething {
			ringBuffer.Add(in)
		}

		if heardSomething {
			if startTime.IsZero() {
				startTime = time.Now()
			}

			for _, sample := range in {
				intBuffer = append(intBuffer, int(sample))
			}

			if maxTime != 0 {
				if time.Since(startTime) > maxTime {
					break
				}
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
