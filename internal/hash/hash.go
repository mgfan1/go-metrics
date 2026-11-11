// Package hash подписывает данные HMAC-SHA256 и проверяет подписи.
package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Header - имя HTTP-заголовка с подписью тела.
const Header = "HashSHA256"

// Sign возвращает подпись data ключом key в hex.
func Sign(data []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// Valid проверяет, что got является подписью data ключом key.
func Valid(data []byte, key, got string) bool {
	want, err := hex.DecodeString(got)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hmac.Equal(mac.Sum(nil), want)
}
