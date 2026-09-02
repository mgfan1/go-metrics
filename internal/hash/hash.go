package hash

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const Header = "HashSHA256"

func Sign(data []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func Valid(data []byte, key, got string) bool {
	want, err := hex.DecodeString(got)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	return hmac.Equal(mac.Sum(nil), want)
}
