package modules

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log"
)

type Crypto struct {
	Block cipher.Block
}

func NewCrypto(key []byte) *Crypto {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}

	return &Crypto{
		Block: block,
	}
}

func (c *Crypto) Encrypt(value string) (result *string, err error) {
	text := []byte(value)

	b := base64.StdEncoding.EncodeToString(text)
	ciphertext := make([]byte, aes.BlockSize+len(b))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	ctr := cipher.NewCTR(c.Block, iv)
	ctr.XORKeyStream(ciphertext[aes.BlockSize:], []byte(b))

	if ciphertext != nil {
		encrypted := string(ciphertext)

		result = &encrypted
	}

	return
}

func (c *Crypto) Decrypt(value string) (result *string, err error) {
	text := []byte(value)

	if len(text) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}

	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]
	ctr := cipher.NewCTR(c.Block, iv)
	ctr.XORKeyStream(text, text)
	data, err := base64.StdEncoding.DecodeString(string(text))

	if data != nil {
		decrypted := string(data)

		result = &decrypted
	}

	return
}
