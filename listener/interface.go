package listener

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
