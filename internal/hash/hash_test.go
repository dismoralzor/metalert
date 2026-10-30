package hash

import "testing"

func TestCompute_Deterministic(t *testing.T) {
	data := []byte(`{"id":"x","type":"gauge","value":1}`)

	got1 := Compute(data, "secret")
	got2 := Compute(data, "secret")
	if got1 != got2 {
		t.Errorf("Compute() не детерминирован: %q != %q", got1, got2)
	}
	if got1 == "" {
		t.Error("Compute() вернул пустую строку")
	}
}

func TestCompute_DifferentKeysDifferentHash(t *testing.T) {
	data := []byte("payload")

	if Compute(data, "key1") == Compute(data, "key2") {
		t.Error("Compute() дал одинаковый хеш для разных ключей")
	}
}

func TestValid(t *testing.T) {
	data := []byte(`[{"id":"x","type":"gauge","value":1}]`)
	key := "secret"
	sum := Compute(data, key)

	tests := []struct {
		name string
		data []byte
		key  string
		got  string
		want bool
	}{
		{"matching signature", data, key, sum, true},
		{"tampered body", []byte("other data"), key, sum, false},
		{"wrong key", data, "wrong-key", sum, false},
		{"garbage hex", data, key, "not-hex-at-all", false},
		{"empty got", data, key, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.data, tt.key, tt.got); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Пустой ключ - не особый случай для самой функции: Compute/Valid просто считают
// HMAC с пустым ключом. "Ничего не считаем" при пустом ключе - это ответственность
// вызывающей стороны (middleware/агент), а не этого пакета.
func TestValid_EmptyKeyStillComputesConsistently(t *testing.T) {
	data := []byte("payload")
	sum := Compute(data, "")

	if !Valid(data, "", sum) {
		t.Error("Valid() = false для собственной подписи с пустым ключом")
	}
}
