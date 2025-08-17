package contract

type State interface {
	Set(key, value string)
	Get(key string) *string
	Delete(key string)
}

// singleton state used everywhere
var state State

func InitState(localDebug bool) {
	if localDebug {
		state = NewMockState()
	} else {
		state = WasmState{}
	}
}

func getState() State {
	return state
}
