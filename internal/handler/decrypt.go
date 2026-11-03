package handler

import (
	"bytes"
	"crypto/rsa"
	"io"
	"net/http"

	"github.com/dismoralzor/metalert/internal/crypto"
)

// encryptedHeader - заголовок-признак того, что тело запроса зашифровано
// гибридной схемой (см. internal/crypto). Агент ставит его последним шагом.
const encryptedHeader = "X-Content-Encrypted"

// DecryptMiddleware - САМЫЙ ВНЕШНИЙ middleware: расшифровывает тело до того, как
// его увидят gzip, проверка подписи и хендлеры. Подключается ПЕРВЫМ в цепочке,
// чтобы ниже по стеку запрос выглядел ровно так же, как незашифрованный.
//
// Если приватный ключ не задан (priv == nil) либо запрос не помечен заголовком
// encryptedHeader - middleware полностью прозрачен (обратная совместимость:
// незашифрованные запросы работают и при загруженном ключе).
func DecryptMiddleware(priv *rsa.PrivateKey) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(encryptedHeader) == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Запрос зашифрован, а расшифровать нечем - это ошибка клиента.
			if priv == nil {
				http.Error(w, "encrypted request but server has no crypto key", http.StatusBadRequest)
				return
			}

			payload, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			r.Body.Close()

			plain, err := crypto.Decrypt(priv, payload)
			if err != nil {
				http.Error(w, "failed to decrypt body", http.StatusBadRequest)
				return
			}

			// Подменяем тело расшифрованным - дальше по цепочке запрос неотличим
			// от незашифрованного. Признак шифрования снимаем.
			r.Body = io.NopCloser(bytes.NewReader(plain))
			r.ContentLength = int64(len(plain))
			r.Header.Del(encryptedHeader)

			next.ServeHTTP(w, r)
		})
	}
}
