package main

import "os"

// shutdown вызывает os.Exit НЕ в main - это разрешено.
func shutdown(code int) {
	os.Exit(code)
}

func main() {
	shutdown(0)
}
