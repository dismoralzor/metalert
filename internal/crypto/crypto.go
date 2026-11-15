// Package crypto реализует гибридное шифрование трафика агент->сервер.
//
// Чистый RSA не годится: в RSA-4096 + OAEP-SHA256 влезает лишь ~446 байт, а
// батч метрик намного больше. Поэтому тело шифруется случайным AES-256-GCM
// ключом, а сам AES-ключ шифруется RSA-OAEP. Формат payload:
//
//	[2 байта big-endian: длина зашифрованного AES-ключа]
//	[зашифрованный AES-ключ]
//	[nonce, 12 байт]
//	[AES-GCM ciphertext вместе с тегом]
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// aesKeySize - размер ключа для AES-256 (байт).
const aesKeySize = 32

// Encrypt шифрует plaintext гибридной схемой публичным ключом pub и возвращает
// самодостаточный payload (см. формат в описании пакета).
func Encrypt(pub *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	aesKey := make([]byte, aesKeySize)
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("crypto: генерация AES-ключа: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: инициализация AES: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: инициализация GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: генерация nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	encKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, aesKey, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: шифрование AES-ключа: %w", err)
	}

	out := make([]byte, 0, 2+len(encKey)+len(nonce)+len(ciphertext))
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(encKey)))
	out = append(out, lenBuf[:]...)
	out = append(out, encKey...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// Decrypt выполняет операцию, обратную Encrypt, приватным ключом priv. На любом
// обрезанном или повреждённом payload возвращает ошибку, а не паникует.
func Decrypt(priv *rsa.PrivateKey, payload []byte) ([]byte, error) {
	if len(payload) < 2 {
		return nil, errors.New("crypto: payload короче заголовка длины")
	}
	keyLen := int(binary.BigEndian.Uint16(payload[:2]))
	rest := payload[2:]
	if len(rest) < keyLen {
		return nil, errors.New("crypto: payload обрезан на зашифрованном ключе")
	}
	encKey := rest[:keyLen]
	rest = rest[keyLen:]

	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, encKey, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: расшифровка AES-ключа: %w", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: инициализация AES: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: инициализация GCM: %w", err)
	}

	ns := gcm.NonceSize()
	if len(rest) < ns {
		return nil, errors.New("crypto: payload обрезан на nonce")
	}
	nonce := rest[:ns]
	ciphertext := rest[ns:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: расшифровка тела: %w", err)
	}
	return plaintext, nil
}

// LoadPublicKey читает RSA-публичный ключ из PEM-файла. Поддерживаются три формы
// блока: "PUBLIC KEY" (PKIX), "RSA PUBLIC KEY" (PKCS1) и "CERTIFICATE" (ключ
// берётся из сертификата).
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}

	switch block.Type {
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: разбор PKIX-ключа: %w", err)
		}
		return asRSAPublic(key)
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: разбор сертификата: %w", err)
		}
		return asRSAPublic(cert.PublicKey)
	default:
		return nil, fmt.Errorf("crypto: неподдерживаемый тип PEM-блока %q", block.Type)
	}
}

// LoadPrivateKey читает RSA-приватный ключ из PEM-файла. Поддерживаются блоки
// "RSA PRIVATE KEY" (PKCS1) и "PRIVATE KEY" (PKCS8).
func LoadPrivateKey(path string) (*rsa.PrivateKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("crypto: разбор PKCS8-ключа: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("crypto: ключ не RSA (%T)", key)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("crypto: неподдерживаемый тип PEM-блока %q", block.Type)
	}
}

// readPEM читает файл и декодирует первый PEM-блок.
func readPEM(path string) (*pem.Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: чтение %s: %w", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("crypto: в %s нет PEM-блока", path)
	}
	return block, nil
}

// asRSAPublic приводит разобранный ключ к *rsa.PublicKey или возвращает ошибку.
func asRSAPublic(key any) (*rsa.PublicKey, error) {
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("crypto: ключ не RSA (%T)", key)
	}
	return rsaKey, nil
}
