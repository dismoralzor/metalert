package crypto_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/dismoralzor/metalert/internal/crypto"
)

// newKey генерирует тестовую RSA-пару (2048 бит - быстрее 4096 для тестов).
func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return key
}

// writePEM записывает PEM-блок во временный файл и возвращает путь.
func writePEM(t *testing.T, name, blockType string, der []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := newKey(t)
	plaintext := []byte("метрики: Alloc=123, PollCount=5")

	payload, err := crypto.Encrypt(&key.PublicKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := crypto.Decrypt(key, payload)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip: got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecrypt_LargePayload(t *testing.T) {
	// 200 КБ - на таком объёме чистый RSA упал бы, а гибридная схема работает.
	key := newKey(t)
	plaintext := bytes.Repeat([]byte("abcd1234"), 25*1024) // 200 КБ

	payload, err := crypto.Encrypt(&key.PublicKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := crypto.Decrypt(key, payload)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("large payload round-trip mismatch (len got=%d want=%d)", len(got), len(plaintext))
	}
}

func TestEncryptDecrypt_EmptyPayload(t *testing.T) {
	key := newKey(t)

	payload, err := crypto.Encrypt(&key.PublicKey, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := crypto.Decrypt(key, payload)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty payload: got %d bytes, want 0", len(got))
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key := newKey(t)
	other := newKey(t)

	payload, err := crypto.Encrypt(&key.PublicKey, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := crypto.Decrypt(other, payload); err == nil {
		t.Error("Decrypt другим ключом должен вернуть ошибку")
	}
}

func TestDecrypt_CorruptedPayload(t *testing.T) {
	key := newKey(t)
	payload, err := crypto.Encrypt(&key.PublicKey, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	cases := map[string][]byte{
		"пустой":              {},
		"только 1 байт":       {0x00},
		"обрезан на ключе":    payload[:5],
		"испорчен хвост":      flip(payload),
		"обрезан на середине": payload[:len(payload)-5],
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			// Главное - не паника; ошибку ждём почти всегда.
			if _, err := crypto.Decrypt(key, bad); err == nil {
				t.Errorf("ожидалась ошибка на повреждённом payload %q", name)
			}
		})
	}
}

// flip портит последний байт payload (ломает GCM-тег).
func flip(b []byte) []byte {
	out := append([]byte(nil), b...)
	out[len(out)-1] ^= 0xFF
	return out
}

func TestLoadPublicKey_Forms(t *testing.T) {
	key := newKey(t)

	pkix, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIX: %v", err)
	}
	pkcs1 := x509.MarshalPKCS1PublicKey(&key.PublicKey)

	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	forms := []struct {
		name      string
		blockType string
		der       []byte
	}{
		{"PKIX", "PUBLIC KEY", pkix},
		{"PKCS1", "RSA PUBLIC KEY", pkcs1},
		{"CERTIFICATE", "CERTIFICATE", certDER},
	}
	for _, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			path := writePEM(t, "pub.pem", f.blockType, f.der)
			pub, err := crypto.LoadPublicKey(path)
			if err != nil {
				t.Fatalf("LoadPublicKey: %v", err)
			}
			if pub.N.Cmp(key.PublicKey.N) != 0 {
				t.Error("загруженный ключ не совпадает с исходным")
			}
		})
	}
}

func TestLoadPrivateKey_Forms(t *testing.T) {
	key := newKey(t)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8: %v", err)
	}
	pkcs1 := x509.MarshalPKCS1PrivateKey(key)

	forms := []struct {
		name      string
		blockType string
		der       []byte
	}{
		{"PKCS1", "RSA PRIVATE KEY", pkcs1},
		{"PKCS8", "PRIVATE KEY", pkcs8},
	}
	for _, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			path := writePEM(t, "priv.pem", f.blockType, f.der)
			priv, err := crypto.LoadPrivateKey(path)
			if err != nil {
				t.Fatalf("LoadPrivateKey: %v", err)
			}
			if priv.N.Cmp(key.N) != 0 {
				t.Error("загруженный ключ не совпадает с исходным")
			}
		})
	}
}

func TestLoadKey_Errors(t *testing.T) {
	t.Run("несуществующий путь", func(t *testing.T) {
		if _, err := crypto.LoadPublicKey(filepath.Join(t.TempDir(), "nope.pem")); err == nil {
			t.Error("ожидалась ошибка для несуществующего файла")
		}
	})
	t.Run("мусор вместо PEM", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "garbage.pem")
		if err := os.WriteFile(path, []byte("это не PEM"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := crypto.LoadPrivateKey(path); err == nil {
			t.Error("ожидалась ошибка для мусора вместо PEM")
		}
	})
}
