package util

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/config"
	"github.com/gofiber/fiber/v2/middleware/encryptcookie"
)

const domain = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func CreateKey(len int) (string, error) {
	// TODO: provided that CreateTripCode still uses sha512, the max len can be 128 at most.
	if len > 128 {
		return "", MakeError(errors.New("len is greater than 128"), "CreateKey")
	}

	str := CreateTripCode(RandomID(len))
	return str[:len], nil
}

func CreateTripCode(input string) string {
	out := sha512.Sum512([]byte(input))

	return hex.EncodeToString(out[:])
}

func GetCookieKey() (string, error) {
	if config.CookieKey == "" {
		var file *os.File
		var err error

		if file, err = os.OpenFile("fchan.cfg", os.O_APPEND|os.O_WRONLY, 0644); err != nil {
			return "", MakeError(err, "GetCookieKey")
		}

		defer file.Close()

		config.CookieKey = encryptcookie.GenerateKey()
		file.WriteString("\ncookiekey:" + config.CookieKey)
	}

	return config.CookieKey, nil
}

func RandomID(size int) string {
	newID := strings.Builder{}
	newID.Grow(size)
	max := big.NewInt(int64(len(domain)))

	for i := 0; i < size; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback to time-based value using domain charset if crypto/rand fails
			config.Log.Printf("RandomID: crypto/rand failed: %v", err)
			timestamp := time.Now().UnixNano()
			// Use timestamp to generate deterministic but unique values
			for i := 0; i < size; i++ {
				pos := (timestamp >> (i * 4)) % int64(len(domain))
				newID.WriteByte(domain[pos])
			}
			return newID.String()
		}
		newID.WriteByte(domain[n.Int64()])
	}

	return newID.String()
}
