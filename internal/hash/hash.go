// Package hash считает и проверяет подпись тела запроса/ответа по HMAC-SHA256.
package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Compute считает HMAC-SHA256(data, key) и возвращает его в виде hex-строки -
// именно так ожидает заголовок HashSHA256.
func Compute(data []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// Valid пересчитывает подпись для data и сравнивает с got (hex-строка из заголовка).
// Сравнение через hmac.Equal по декодированным байтам, а не через == строк -
// защита от timing-атак (== сравнивает байт-за-байтом и выходит на первом
// несовпадении, что позволяет по времени ответа угадывать подпись посимвольно).
func Valid(data []byte, key, got string) bool {
	gotBytes, err := hex.DecodeString(got)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	want := mac.Sum(nil)

	return hmac.Equal(want, gotBytes)
}
