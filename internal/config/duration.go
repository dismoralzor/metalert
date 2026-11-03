// Package config содержит общие типы для JSON-конфигурации сервера и агента.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Duration - длительность для JSON-конфига. В файле принимается в двух формах:
//   - строка формата time.ParseDuration ("1s", "300ms", "2m");
//   - число, трактуемое как секунды (для совместимости).
//
// Отрицательные значения запрещены. Обёртка над time.Duration нужна ради
// собственного UnmarshalJSON.
type Duration struct {
	time.Duration
}

// UnmarshalJSON разбирает длительность из строки или числа (секунды).
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch x := v.(type) {
	case string:
		dur, err := time.ParseDuration(x)
		if err != nil {
			return fmt.Errorf("неверная длительность %q: %w", x, err)
		}
		if dur < 0 {
			return fmt.Errorf("отрицательная длительность %q", x)
		}
		d.Duration = dur
		return nil
	case float64:
		if x < 0 {
			return fmt.Errorf("отрицательная длительность %v", x)
		}
		d.Duration = time.Duration(x * float64(time.Second))
		return nil
	default:
		return errors.New("длительность должна быть строкой или числом секунд")
	}
}
