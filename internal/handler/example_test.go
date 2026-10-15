package handler_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/repository"
)

// newExampleServer поднимает тестовый HTTP-сервер с основными ручками поверх
// in-memory хранилища (без БД) - ровно те маршруты, что регистрирует сервер.
func newExampleServer() *httptest.Server {
	storage := repository.NewMemStorage()

	r := chi.NewRouter()
	r.Post("/update/{type}/{name}/{value}", handler.NewUpdateHandler(storage, nil).Update)
	r.Get("/value/{type}/{name}", handler.NewValueHandler(storage).Value)
	r.Post("/update/", handler.NewUpdateJSONHandler(storage, nil).Update)
	r.Post("/value/", handler.NewValueJSONHandler(storage).Value)

	return httptest.NewServer(r)
}

// post шлёт POST и возвращает тело ответа строкой (вспомогательная для примеров).
func post(url, contentType, body string) string {
	resp, err := http.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return string(out)
}

// get шлёт GET и возвращает тело ответа строкой.
func get(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return string(out)
}

// Example демонстрирует базовый цикл: сохранить gauge текстовым запросом и
// прочитать его обратно.
func Example() {
	srv := newExampleServer()
	defer srv.Close()

	post(srv.URL+"/update/gauge/Alloc/42.5", "text/plain", "")
	fmt.Println(get(srv.URL + "/value/gauge/Alloc"))

	// Output:
	// 42.5
}

// ExampleUpdateHandler_gauge сохраняет gauge по текстовому протоколу и читает
// значение обратно через GET /value.
func ExampleUpdateHandler_gauge() {
	srv := newExampleServer()
	defer srv.Close()

	post(srv.URL+"/update/gauge/Temperature/23.5", "text/plain", "")
	fmt.Println(get(srv.URL + "/value/gauge/Temperature"))

	// Output:
	// 23.5
}

// ExampleUpdateHandler_counter показывает накопление counter: два POST с delta
// суммируются, GET /value возвращает сумму.
func ExampleUpdateHandler_counter() {
	srv := newExampleServer()
	defer srv.Close()

	post(srv.URL+"/update/counter/PollCount/10", "text/plain", "")
	post(srv.URL+"/update/counter/PollCount/5", "text/plain", "")
	fmt.Println(get(srv.URL + "/value/counter/PollCount"))

	// Output:
	// 15
}

// ExampleUpdateJSONHandler сохраняет метрику JSON-запросом и печатает ответ -
// ту же метрику с заполненным полем value.
func ExampleUpdateJSONHandler() {
	srv := newExampleServer()
	defer srv.Close()

	// Ответ JSON-хендлера уже заканчивается переводом строки.
	fmt.Print(post(srv.URL+"/update/", "application/json",
		`{"id":"Temp","type":"gauge","value":23.5}`))

	// Output:
	// {"id":"Temp","type":"gauge","value":23.5}
}

// ExampleValueJSONHandler накапливает counter и запрашивает его значение
// JSON-запросом: в ответе заполнено поле delta.
func ExampleValueJSONHandler() {
	srv := newExampleServer()
	defer srv.Close()

	post(srv.URL+"/update/counter/Requests/40", "text/plain", "")
	post(srv.URL+"/update/counter/Requests/2", "text/plain", "")

	fmt.Print(post(srv.URL+"/value/", "application/json",
		`{"id":"Requests","type":"counter"}`))

	// Output:
	// {"id":"Requests","type":"counter","delta":42}
}
