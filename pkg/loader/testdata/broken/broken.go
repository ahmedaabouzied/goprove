package broken

// This file has intentional errors to test the loader's error handling.

func Bad() int {
	return "not an int" // type error: string cannot be returned as int
}
