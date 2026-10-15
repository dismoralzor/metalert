package audit

import (
	"encoding/json"
	"os"
	"sync"
)

// FileObserver пишет события аудита в файл по одной JSON-строке на событие
// (формат JSONL). Собственный мьютекс защищает от переплетения байтов, если
// два параллельных запроса вызовут Notify одновременно.
type FileObserver struct {
	mu   sync.Mutex
	path string
}

// NewFileObserver создаёт приёмник, дописывающий события в файл path.
func NewFileObserver(path string) *FileObserver {
	return &FileObserver{path: path}
}

// Notify сериализует событие в JSON и дописывает строку + '\n' в конец файла.
// O_APPEND гарантирует запись в хвост даже при нескольких открытиях файла,
// мьютекс - атомарность пары "сериализация + запись" внутри процесса.
func (o *FileObserver) Notify(e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	o.mu.Lock()
	defer o.mu.Unlock()

	f, err := os.OpenFile(o.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}
