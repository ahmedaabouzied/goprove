package testdata

// ---------------------------------------------------------------------------
// Coverage helper fixtures
// These functions exist solely to exercise uncovered code paths in tests.
// ---------------------------------------------------------------------------

// ZeroParamFunc has zero params but is called — exercises nParams==0 skip.
func ZeroParamFunc() int {
	return 42
}

// CallerOfZeroParam calls ZeroParamFunc.
func CallerOfZeroParam() int {
	return ZeroParamFunc()
}

// GoRoutineInterfaceDispatch exercises the go + interface dispatch path.
type Worker interface {
	Work()
}

func LaunchWorker(w Worker) {
	go w.Work() // go + interface dispatch → StaticCallee() returns nil
}
