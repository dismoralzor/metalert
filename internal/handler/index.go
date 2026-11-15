package handler

import (
	"bytes"
	"html"
	"net/http"
	"strconv"
	"sync"

	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// indexScratch - переиспользуемые между запросами буферы рендера: сам буфер
// страницы и вспомогательный слайс num для форматирования чисел через
// strconv.Append* (чтобы не аллоцировать строку на каждую метрику).
type indexScratch struct {
	buf bytes.Buffer
	num []byte
}

// indexBufPool переиспользует indexScratch между запросами: страница собирается
// заново на каждый GET /, но буферы аллоцировать с нуля каждый раз не нужно -
// на горячем пути это давало основную долю аллокаций.
var indexBufPool = sync.Pool{
	New: func() any { return new(indexScratch) },
}

// IndexHandler обслуживает GET / - HTML-страницу со списком всех метрик.
type IndexHandler struct {
	storage repository.Storage
}

// NewIndexHandler создаёт обработчик страницы со списком метрик поверх storage.
func NewIndexHandler(storage repository.Storage) *IndexHandler {
	return &IndexHandler{storage: storage}
}

// Index рендерит HTML-страницу с текущими gauge- и counter-метриками.
func (h *IndexHandler) Index(w http.ResponseWriter, r *http.Request) {
	metrics := h.storage.Metrics(r.Context())

	// Собираем HTML прямо в переиспользуемый буфер, без промежуточных срезов
	// строк таблицы и без рефлексии html/template. Имена метрик приходят извне,
	// поэтому экранируем их через html.EscapeString (значения числовые -
	// strconv даёт безопасный вывод). Структура страницы прежняя.
	sc := indexBufPool.Get().(*indexScratch)
	sc.buf.Reset()
	defer indexBufPool.Put(sc)
	buf := &sc.buf

	buf.WriteString("<!DOCTYPE html>\n<html>\n<head><title>Metrics</title></head>\n<body>\n<h1>Metrics</h1>\n<h2>Gauges</h2>\n<ul>\n")
	for _, m := range metrics {
		if m.MType != models.Gauge || m.Value == nil {
			continue
		}
		buf.WriteString("<li>")
		buf.WriteString(html.EscapeString(m.ID))
		buf.WriteString(": ")
		// AppendFloat в переиспользуемый scratch вместо FormatFloat: не аллоцирует
		// строку на каждую метрику.
		sc.num = strconv.AppendFloat(sc.num[:0], *m.Value, 'f', -1, 64)
		buf.Write(sc.num)
		buf.WriteString("</li>\n")
	}
	buf.WriteString("</ul>\n<h2>Counters</h2>\n<ul>\n")
	for _, m := range metrics {
		if m.MType != models.Counter || m.Delta == nil {
			continue
		}
		buf.WriteString("<li>")
		buf.WriteString(html.EscapeString(m.ID))
		buf.WriteString(": ")
		sc.num = strconv.AppendInt(sc.num[:0], *m.Delta, 10)
		buf.Write(sc.num)
		buf.WriteString("</li>\n")
	}
	buf.WriteString("</ul>\n</body>\n</html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
