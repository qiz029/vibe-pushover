package pushover

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
)

func encryptMessageFields(form url.Values, keyHex string) error {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("Pushover encryption key must be a 64-character hex string")
	}
	for _, field := range []string{"message", "title", "url", "url_title"} {
		plaintext := form.Get(field)
		if field != "message" && plaintext == "" {
			form.Del(field)
			continue
		}
		encrypted, err := encryptField(rand.Reader, key, plaintext)
		if err != nil {
			return fmt.Errorf("encrypt Pushover %s: %w", field, err)
		}
		form.Set(field, encrypted)
	}
	form.Set("encrypted", "1")
	return nil
}

func encryptField(random io.Reader, key []byte, plaintext string) (string, error) {
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := writer.Write([]byte(plaintext)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(random, iv); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	data := compressed.Bytes()
	padding := aes.BlockSize - len(data)%aes.BlockSize
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for index := len(data); index < len(padded); index++ {
		padded[index] = byte(padding)
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	payload := make([]byte, 0, len(iv)+len(ciphertext)+sha256.Size)
	payload = append(payload, iv...)
	payload = append(payload, ciphertext...)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	payload = append(payload, mac.Sum(nil)...)
	return base64.StdEncoding.EncodeToString(payload), nil
}
