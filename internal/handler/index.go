package handler

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/dismoralzor/metalert/internal/repository"
)

// html/template, не text/template: экранирует значения при вставке в HTML.
const indexTemplateSrc = `<!DOCTYPE html>
<html>
<head><title>Metrics</title></head>
<body>
<h1>Metrics</h1>
<h2>Gauges</h2>
<ul>
{{range $name, $value := .Gauges}}<li>{{$name}}: {{$value}}</li>
{{end}}</ul>
<h2>Counters</h2>
<ul>
{{range $name, $value := .Counters}}<li>{{$name}}: {{$value}}</li>
{{end}}</ul>
</body>
</html>`

type IndexHandler struct {
	storage repository.Storage
	tmpl    *template.Template
}

func NewIndexHandler(storage repository.Storage) *IndexHandler {
	// Must паникует при невалидном шаблоне - это баг в коде, падать при старте
	// приложения правильнее, чем на каждый запрос.
	tmpl := template.Must(template.New("index").Parse(indexTemplateSrc))
	return &IndexHandler{storage: storage, tmpl: tmpl}
}

func (h *IndexHandler) Index(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Gauges   map[string]float64
		Counters map[string]int64
	}{
		Gauges:   h.storage.Gauges(),
		Counters: h.storage.Counters(),
	}

	// Рендерим в буфер: если Execute упадёт на середине шаблона, w ещё не увидит
	// ни байта, и http.Error сможет честно поставить 500 (иначе первый Write
	// уже закоммитит статус 200).
	var buf bytes.Buffer
	if err := h.tmpl.Execute(&buf, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
