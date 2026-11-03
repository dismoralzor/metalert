package crypto_test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/dismoralzor/metalert/internal/crypto"
)

// ExampleEncrypt шифрует и расшифровывает сообщение: после round-trip
// расшифрованный текст совпадает с исходным.
func ExampleEncrypt() {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	payload, _ := crypto.Encrypt(&key.PublicKey, []byte("hello"))
	plain, _ := crypto.Decrypt(key, payload)

	fmt.Println(string(plain))

	// Output:
	// hello
}
