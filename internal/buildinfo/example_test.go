package buildinfo_test

import (
	"os"

	"github.com/dismoralzor/metalert/internal/buildinfo"
)

// ExamplePrint показывает вывод с заданными значениями и с пустыми (подстановка
// "N/A").
func ExamplePrint() {
	buildinfo.Print(os.Stdout, "v1.0.1", "2026/10/31 12:00:00", "abc1234")
	buildinfo.Print(os.Stdout, "", "", "")

	// Output:
	// Build version: v1.0.1
	// Build date: 2026/10/31 12:00:00
	// Build commit: abc1234
	// Build version: N/A
	// Build date: N/A
	// Build commit: N/A
}
