package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Без RouteContext в контексте запроса chi.URLParam всегда возвращает "" -
// в реальном сервере его кладёт chi.Router при матчинге маршрута.
func withChiURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
