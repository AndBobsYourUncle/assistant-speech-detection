package speech_extraction

import (
	"assistant-speech-detection/speech_extraction/voice_detection"
	"assistant-speech-detection/speech_to_text"
	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/gordonklaus/portaudio"
	"github.com/spf13/afero"
	"github.com/zenwerk/go-wave"
	"log"
	"strconv"
	"time"
)

type voiceImpl struct {
	fileSys      afero.Fs
	audioRunning bool
	model        whisper.Model
}

func NewVoice(fileSys afero.Fs, model whisper.Model) *voiceImpl {
	return &voiceImpl{
		fileSys: fileSys,
		model:   model,
	}
}

func (v *voiceImpl) Listen() error {
	err := v.initAudio()
	if err != nil {
		return err
	}

	defer v.freeAudio()

	waveFilename, err := v.listenIntoBuffer(DefaultQuietTime)
	if err != nil {
		log.Fatal(err)
	}

	err = speech_to_text.Process(v.fileSys, v.model, *waveFilename)
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

const DefaultQuietTime = time.Millisecond * 200

func (v *voiceImpl) listenIntoBuffer(quietTime time.Duration) (*string, error) {
	in := make([]int16, 8196)
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

	vad := voice_detection.NewVAD(len(in))

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
		return nil, err
	}

	defer waveWriter.Close()

	for {
		err = stream.Read()
		if err != nil {
			return nil, err
		}

		// TODO we need a circular buffer here to prepend the first bit detected, or else we miss the first few words
		//if heardSomething {
		_, err = waveWriter.WriteSample16(in)
		if err != nil {
			return nil, err
		}
		//}

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
			}

			lastFlux = flux
		}
	}

	err = stream.Stop()
	if err != nil {
		return nil, err
	}

	return &waveFilename, nil
}
