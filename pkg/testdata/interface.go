package testdata

type Printer interface {
	Print(input string)
}

func PrintSomething(p Printer) {
	p.Print("something")
}

func Handler() {
	PrintSomething(nil) // Error report: Definitely nil.
}
