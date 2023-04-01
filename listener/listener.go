package listener

import (
	"assistant-speech-detection/listener/voice_activity_detection"
	"assistant-speech-detection/ring_buffer"
	"assistant-speech-detection/speech_to_text"
	"fmt"
	"github.com/go-audio/audio"
	"github.com/gordonklaus/portaudio"
	"log"
	"strconv"
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
	deviceID        string
	audioRunning    bool
	sttEngine       speech_to_text.Interface
	triggeredAction ListenAction
	interrupt       bool
	inBuffer        []int16
	stream          *portaudio.Stream
}

type Config struct {
	DeviceID  string
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
		deviceID:        cfg.DeviceID,
		sttEngine:       cfg.STTEngine,
		triggeredAction: ListenActionWake,
		inBuffer:        make([]int16, bufferSize),
	}, nil
}

func (v *voiceImpl) ListenLoop() error {
	err := v.initAudio()
	if err != nil {
		return err
	}

	defer v.freeAudio()

	devices, err := portaudio.Devices()
	if err != nil {
		return err
	}

	for idx, device := range devices {
		if device.MaxInputChannels > 0 {
			log.Printf("device %d: %+v\n", idx, device.Name)
		}
	}

	selectedDevice, err := portaudio.DefaultInputDevice()
	if err != nil {
		return err
	}

	log.Printf("default device: %+v\n", selectedDevice.Name)

	if v.deviceID != "" {
		deviceID, convErr := strconv.Atoi(v.deviceID)
		if convErr != nil {
			return convErr
		}

		if deviceID >= len(devices) {
			return fmt.Errorf("invalid device id")
		}

		selectedDevice = devices[deviceID]
	}

	log.Printf("chosen device: %+v\n", selectedDevice.Name)

	log.Printf("sample rate: %d\n", selectedDevice.DefaultSampleRate)

	p := portaudio.LowLatencyParameters(selectedDevice, nil)
	p.Input.Channels = 1
	p.Output.Channels = 0
	p.SampleRate = 16000
	p.FramesPerBuffer = len(v.inBuffer)

	stream, err := portaudio.OpenStream(p, v.inBuffer)
	if err != nil {
		return err
	}

	v.stream = stream

	err = stream.Start()
	if err != nil {
		return err
	}

	defer stream.Stop()

	defer stream.Close()

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
	var (
		heardSomething bool
		quiet          bool
		quietStart     time.Time
		lastFlux       float64
	)

	vad := voice_activity_detection.New(len(v.inBuffer))

	ringBuffer := ring_buffer.New(bufferSize)

	intBuffer := make([]int, 0)

	var startTime time.Time

	for {
		if v.interrupt {
			v.interrupt = false

			// interrupt the current listening loop, return empty buffer
			return &audio.IntBuffer{
				Format: &audio.Format{
					NumChannels: 1,
					SampleRate:  16000,
				},
				Data:           []int{},
				SourceBitDepth: 16,
			}, nil
		}

		err := v.stream.Read()
		if err != nil {
			return nil, err
		}

		// keep a buffer of the first bit of audio before detection
		if !heardSomething {
			ringBuffer.Add(v.inBuffer)
		}

		if heardSomething {
			if startTime.IsZero() {
				startTime = time.Now()
			}

			for _, sample := range v.inBuffer {
				intBuffer = append(intBuffer, int(sample))
			}

			if maxTime != 0 {
				if time.Since(startTime) > maxTime {
					break
				}
			}
		}

		flux := vad.Flux(v.inBuffer)

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
				for _, sample := range v.inBuffer {
					intBuffer = append(intBuffer, int(sample))
				}
			}

			lastFlux = flux
		}
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
