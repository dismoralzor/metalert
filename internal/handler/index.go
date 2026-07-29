package handler

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/dismoralzor/metalert/internal/model"
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
{{range .Gauges}}<li>{{.Name}}: {{.Value}}</li>
{{end}}</ul>
<h2>Counters</h2>
<ul>
{{range .Counters}}<li>{{.Name}}: {{.Value}}</li>
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
	// Metrics() отдаёт единый список без разделения по типу - делим его тут,
	// а не в Storage, потому что это забота презентации (две секции на странице),
	// а не хранилища.
	var data struct {
		Gauges   []repository.Metric
		Counters []repository.Metric
	}
	for _, metric := range h.storage.Metrics() {
		switch metric.Type {
		case models.Gauge:
			data.Gauges = append(data.Gauges, metric)
		case models.Counter:
			data.Counters = append(data.Counters, metric)
		}
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
