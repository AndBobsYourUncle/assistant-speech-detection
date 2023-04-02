package listener

/*
typedef unsigned char Uint8;
void OnAudio(void *userdata, Uint8 *stream, int length);
*/
import "C"

import (
	"assistant-speech-detection/clients/ai_bot"
	"assistant-speech-detection/listener/voice_activity_detection"
	"assistant-speech-detection/ring_buffer"
	"assistant-speech-detection/speech_to_text"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-audio/audio"
	"github.com/veandco/go-sdl2/sdl"
)

const (
	quietTimePeriod = time.Millisecond * 200
)

type ListenAction string

const (
	ListenActionWait    ListenAction = "wait"
	ListenActionWake    ListenAction = "wake"
	ListenActionCommand ListenAction = "command"
)

const (
	defaultFrequency = 16000
	defaultFormat    = sdl.AUDIO_S16
	defaultChannels  = 1
	defaultSamples   = 8196
)

var (
	audioC = make(chan []int16, 1)
)

//export OnAudio
func OnAudio(userdata unsafe.Pointer, _stream *C.Uint8, _length C.int) {
	// We need to cast the stream from C uint8 array into Go int16 slice
	length := int(_length) / 2                                                                      // Divide by 2 because a single int16 consists of two uint8
	header := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(_stream)), Len: length, Cap: length} // Build the slice header for our int16 slice
	buf := *(*[]int16)(unsafe.Pointer(&header))                                                     // Use the slice header as int16 slice

	// Copy the audio samples into temporary buffer
	audioSamples := make([]int16, length)
	copy(audioSamples, buf)

	// Send the temporary buffer to our main function via our Go channel
	audioC <- audioSamples
}

type voiceImpl struct {
	deviceID        string
	sttEngine       speech_to_text.Interface
	triggeredAction ListenAction
	interrupt       bool
	inBuffer        []int16
	audioDeviceID   sdl.AudioDeviceID
	exitGracefully  bool
	aiBotClient     ai_bot.AIBotAPI
}

type Config struct {
	DeviceID    string
	STTEngine   speech_to_text.Interface
	AIBotClient ai_bot.AIBotAPI
}

func New(cfg *Config) (Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.STTEngine == nil {
		return nil, fmt.Errorf("sttEngine is nil")
	}

	if cfg.AIBotClient == nil {
		return nil, fmt.Errorf("aiBotClient is nil")
	}

	return &voiceImpl{
		deviceID:        cfg.DeviceID,
		sttEngine:       cfg.STTEngine,
		triggeredAction: ListenActionWake,
		inBuffer:        make([]int16, defaultSamples),
		aiBotClient:     cfg.AIBotClient,
	}, nil
}

func (v *voiceImpl) ListenLoop() error {
	var dev sdl.AudioDeviceID

	// Initialize SDL2
	err := sdl.Init(sdl.INIT_AUDIO)
	if err != nil {
		return err
	}

	defer sdl.Quit()

	numDevices := sdl.GetNumAudioDevices(true)

	log.Printf("num audio devices: %d", numDevices)

	for i := 0; i < numDevices; i++ {
		name := sdl.GetAudioDeviceName(i, true)

		log.Printf("device %d: %s\n", i, name)
	}

	deviceID := 0

	if v.deviceID != "" {
		deviceID, err = strconv.Atoi(v.deviceID)
		if err != nil {
			return err
		}
	}

	// Specify the configuration for our default recording device
	spec := sdl.AudioSpec{
		Freq:     defaultFrequency,
		Format:   defaultFormat,
		Channels: defaultChannels,
		Samples:  defaultSamples,
		Callback: sdl.AudioCallback(C.OnAudio),
	}

	// Open default recording device
	defaultRecordingDeviceName := sdl.GetAudioDeviceName(deviceID, true)
	if dev, err = sdl.OpenAudioDevice(defaultRecordingDeviceName, true, &spec, nil, 0); err != nil {
		return err
	}
	defer sdl.CloseAudioDevice(dev)

	v.audioDeviceID = dev

	log.Printf("starting to listen\n")

	for {
		if v.exitGracefully {
			log.Printf("exiting gracefully\n")

			return nil
		}

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
	log.Printf("expecting a command\n")

	for {
		if v.exitGracefully {
			return nil
		}

		waveBuffer, err := v.listenIntoBuffer(quietTimePeriod, 0)
		if err != nil {
			log.Fatalf("error listening: %v", err)
		}

		if waveBuffer.NumFrames() == 0 {
			continue
		}

		segments, err := v.sttEngine.Process(waveBuffer)
		if err != nil {
			log.Printf("error running model: %v", err)

			return err
		}

		fullCommand := ""

		for _, segment := range segments {
			log.Printf("[%6s->%6s] %s\n",
				segment.Start.Truncate(time.Millisecond), segment.End.Truncate(time.Millisecond), segment.Text)

			fullCommand += segment.Text + " "
		}

		v.triggeredAction = ListenActionWake

		if fullCommand != "" {
			log.Printf("sending command to bot: %s\n", fullCommand)

			resp, sendErr := v.aiBotClient.SendPrompt(context.Background(), fullCommand)
			if sendErr != nil {
				log.Printf("error sending command to bot: %v", sendErr)
			}

			log.Printf("bot response: %s\n", resp)
		}

		return nil
	}
}

func (v *voiceImpl) listenForWakeLoop() error {
	log.Printf("waiting for wake\n")

	for {
		if v.exitGracefully {
			return nil
		}

		waveBuffer, err := v.listenIntoBuffer(quietTimePeriod, time.Millisecond*500)
		if err != nil {
			log.Fatalf("error listening: %v", err)
		}

		if waveBuffer.NumFrames() == 0 {
			continue
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

				return nil
			}
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

	ringBuffer := ring_buffer.New(defaultSamples)

	intBuffer := make([]int, 0)

	var startTime time.Time

	// Start recording audio
	sdl.PauseAudioDevice(v.audioDeviceID, false)

	defer sdl.PauseAudioDevice(v.audioDeviceID, true)

	// Listen to OS signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	for {
		if v.interrupt {
			log.Printf("interrupted\n")

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
		} else {
			select {
			case <-c:
				v.interrupt = true
				v.exitGracefully = true
			case v.inBuffer = <-audioC:
			}
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
