package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	root := flag.String("root", "", "корень модуля для сканирования (по умолчанию ищется вверх по go.mod)")
	flag.Parse()

	dir := *root
	if dir == "" {
		found, err := findModuleRoot()
		if err != nil {
			return err
		}
		dir = found
	}

	return Generate(dir)
}

// findModuleRoot поднимается от текущего каталога вверх по дереву, пока не
// найдёт go.mod. Это позволяет go generate запускать команду из каталога любого
// пакета - генератор всё равно отсканирует весь модуль.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
