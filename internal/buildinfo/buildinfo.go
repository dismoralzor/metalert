// Package buildinfo печатает сведения о сборке (версия, дата, коммит),
// подставляя "N/A" вместо незаданных значений. Значения приходят из переменных
// package main команд, заполняемых линковщиком через -ldflags -X.
package buildinfo

import (
	"fmt"
	"io"
)

// Print выводит в w информацию о сборке, подставляя "N/A" вместо пустых
// значений. Формат - ровно три строки, каждая завершается \n.
func Print(w io.Writer, version, date, commit string) {
	fmt.Fprintf(w, "Build version: %s\n", na(version))
	fmt.Fprintf(w, "Build date: %s\n", na(date))
	fmt.Fprintf(w, "Build commit: %s\n", na(commit))
}

// na возвращает "N/A" для пустой строки и сам аргумент в остальных случаях.
func na(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}
