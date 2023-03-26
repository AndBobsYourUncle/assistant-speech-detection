package speech_extraction

import (
	"assistant-speech-detection/ring_buffer"
	"assistant-speech-detection/speech_extraction/vad"
	"assistant-speech-detection/speech_to_text"
	"fmt"
	"github.com/gordonklaus/portaudio"
	"github.com/spf13/afero"
	"github.com/zenwerk/go-wave"
	"log"
	"strconv"
	"time"
)

const (
	quietTimePeriod = time.Millisecond * 200
	bufferSize      = 8196
)

type voiceImpl struct {
	fileSys      afero.Fs
	audioRunning bool
	sttEngine    speech_to_text.Interface
}

type Config struct {
	FileSys   afero.Fs
	STTEngine speech_to_text.Interface
}

func New(cfg *Config) (Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.FileSys == nil {
		return nil, fmt.Errorf("fileSys is nil")
	}

	if cfg.STTEngine == nil {
		return nil, fmt.Errorf("sttEngine is nil")
	}

	return &voiceImpl{
		fileSys:   cfg.FileSys,
		sttEngine: cfg.STTEngine,
	}, nil
}

func (v *voiceImpl) Listen() error {
	err := v.initAudio()
	if err != nil {
		return err
	}

	defer v.freeAudio()

	waveFilename, err := v.listenIntoBuffer(quietTimePeriod)
	if err != nil {
		log.Fatal(err)
	}

	err = v.sttEngine.Process(waveFilename)
	if err != nil {
		log.Printf("error running model: %v", err)

		return err
	}

	v.Listen()

	return nil
}

func (v *voiceImpl) initAudio() error {
	if !v.audioRunning {
		err := portaudio.Initialize()
		if err != nil {
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

func (v *voiceImpl) listenIntoBuffer(quietTime time.Duration) (string, error) {
	in := make([]int16, bufferSize)
	stream, err := portaudio.OpenDefaultStream(1, 0, 16000, len(in), in)
	if err != nil {
		return "", err
	}

	defer stream.Close()

	err = stream.Start()
	if err != nil {
		return "", err
	}

	var (
		heardSomething bool
		quiet          bool
		quietStart     time.Time
		lastFlux       float64
	)

	vad := vad.New(len(in))

	waveFilename := "test" + strconv.Itoa(int(time.Now().Unix())) + ".wav"

	waveFile, err := v.fileSys.Create(waveFilename)

	param := wave.WriterParam{
		Out:           waveFile,
		Channel:       1,
		SampleRate:    16000,
		BitsPerSample: 16,
	}

	waveWriter, err := wave.NewWriter(param)
	if err != nil {
		return "", err
	}

	defer waveWriter.Close()

	ringBuffer := ring_buffer.New(bufferSize)

	for {
		err = stream.Read()
		if err != nil {
			return "", err
		}

		// keep a buffer of the first bit of audio before detection
		if !heardSomething {
			ringBuffer.Add(in)
		}

		if heardSomething {
			_, err = waveWriter.WriteSample16(in)
			if err != nil {
				return "", err
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

				// write the first bit of the buffer to the wav file
				_, err = waveWriter.WriteSample16(ringBuffer.Read())
				if err != nil {
					return "", err
				}
			}

			lastFlux = flux
		}
	}

	err = stream.Stop()
	if err != nil {
		return "", err
	}

	return waveFilename, nil
}
