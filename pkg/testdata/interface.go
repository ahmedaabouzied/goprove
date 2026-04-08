package testdata

func Handler() {
	PrintSomething(nil) // Error report: Definitely nil.
}

func PrintSomething(p Printer) {
	p.Print("something")
}

type Printer interface {
	Print(input string)
}
