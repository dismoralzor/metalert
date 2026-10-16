package audit

import (
	"encoding/json"
	"os"
	"sync"
)

// FileObserver пишет события аудита в файл по одной JSON-строке на событие
// (формат JSONL). Файл открывается один раз в конструкторе и держится открытым;
// мьютекс защищает от переплетения байтов, если два параллельных запроса вызовут
// Notify одновременно.
type FileObserver struct {
	mu sync.Mutex
	f  *os.File
}

// NewFileObserver открывает (создаёт при отсутствии) файл path на дозапись и
// возвращает приёмник. Ошибку открытия возвращаем сразу - вызывающая сторона
// (main) падает на старте, а не на первом событии.
func NewFileObserver(path string) (*FileObserver, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileObserver{f: f}, nil
}

// Notify сериализует событие в JSON и дописывает строку + '\n' в уже открытый
// файл под мьютексом (O_APPEND гарантирует запись в хвост).
func (o *FileObserver) Notify(e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	o.mu.Lock()
	defer o.mu.Unlock()

	_, err = o.f.Write(data)
	return err
}

// Close закрывает файл. Вызывается Subject.Close при остановке сервера.
func (o *FileObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.f.Close()
}
