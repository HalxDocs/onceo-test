package concurrency

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
)

func testSignedHeaders(secret string, body []byte) http.Header {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	headers := http.Header{}
	headers.Set("X-Paystack-Signature", sig)
	return headers
}

var (
	_ = errors.Is    // keep imports
	_ = fmt.Sprintf
)
