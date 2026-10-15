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

// indexBufPool переиспользует буферы рендера между запросами: страница
// собирается заново на каждый GET /, но сам буфер аллоцировать с нуля каждый
// раз не нужно - на горячем пути это давало основную долю аллокаций.
var indexBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type IndexHandler struct {
	storage repository.Storage
}

func NewIndexHandler(storage repository.Storage) *IndexHandler {
	return &IndexHandler{storage: storage}
}

func (h *IndexHandler) Index(w http.ResponseWriter, r *http.Request) {
	metrics := h.storage.Metrics(r.Context())

	// Собираем HTML прямо в переиспользуемый буфер, без промежуточных срезов
	// строк таблицы и без рефлексии html/template. Имена метрик приходят извне,
	// поэтому экранируем их через html.EscapeString (значения числовые -
	// strconv даёт безопасный вывод). Структура страницы прежняя.
	buf := indexBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer indexBufPool.Put(buf)

	buf.WriteString("<!DOCTYPE html>\n<html>\n<head><title>Metrics</title></head>\n<body>\n<h1>Metrics</h1>\n<h2>Gauges</h2>\n<ul>\n")
	for _, m := range metrics {
		if m.MType != models.Gauge || m.Value == nil {
			continue
		}
		buf.WriteString("<li>")
		buf.WriteString(html.EscapeString(m.ID))
		buf.WriteString(": ")
		buf.WriteString(strconv.FormatFloat(*m.Value, 'f', -1, 64))
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
		buf.WriteString(strconv.FormatInt(*m.Delta, 10))
		buf.WriteString("</li>\n")
	}
	buf.WriteString("</ul>\n</body>\n</html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
