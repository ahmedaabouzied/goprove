package testdata

// CallerOfZeroParam calls ZeroParamFunc.
func CallerOfZeroParam() int {
	return ZeroParamFunc()
}

func LaunchWorker(w Worker) {
	go w.Work() // go + interface dispatch → StaticCallee() returns nil
}

// ---------------------------------------------------------------------------
// Coverage helper fixtures
// These functions exist solely to exercise uncovered code paths in tests.
// ---------------------------------------------------------------------------
// ZeroParamFunc has zero params but is called — exercises nParams==0 skip.
func ZeroParamFunc() int {
	return 42
}

// GoRoutineInterfaceDispatch exercises the go + interface dispatch path.
type Worker interface {
	Work()
}
