package speech_extraction

type Interface interface {
	ListenLoop() error
	HaltListening()
	ListenForWake()
	ListenForCommand()
}

type ControlInterface interface {
	HaltListening()
	ListenForWake()
	ListenForCommand()
}
