package main

import (
	"os"

	xos "os"
)

func main() {
	os.Exit(1)  // want "прямой вызов os.Exit в функции main пакета main"
	xos.Exit(2) // want "прямой вызов os.Exit в функции main пакета main"
}
