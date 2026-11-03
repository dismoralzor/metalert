// Package lib - не пакет main, поэтому даже функция main с os.Exit не
// считается нарушением.
package lib

import "os"

func main() {
	os.Exit(1)
}
