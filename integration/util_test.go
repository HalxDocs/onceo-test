package integration

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
)

func testSignHMACSHA512(secret, body []byte) string {
	mac := hmac.New(sha512.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func testSHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
