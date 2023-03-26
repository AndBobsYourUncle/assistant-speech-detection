package speech_to_text

type Interface interface {
	Process(waveFilename string) error
}
