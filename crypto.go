package nethub

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rc4"
	"encoding/base64"
	"fmt"
)

// ecdh+rc4
type Crypto struct {
	priKey  *ecdh.PrivateKey
	pubKey  *ecdh.PublicKey
	encoder *rc4.Cipher
	decoder *rc4.Cipher
}

func NewCrypto() *Crypto {
	priKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	pubKey := priKey.PublicKey()
	return &Crypto{priKey: priKey, pubKey: pubKey}
}

// 与不安全的远程协商通信secret，返回pubkey
func (c *Crypto) ComputeSecret(remotePubBase64 string) (string, error) {
	remotePubKey, err := base64.StdEncoding.DecodeString(remotePubBase64)
	if err != nil {
		return "", err
	}
	rpubkey, err := ecdh.P256().NewPublicKey(remotePubKey)
	if err != nil {
		return "", fmt.Errorf("ecdh compute secrete err:%v", err.Error())
	}
	secret, err := c.priKey.ECDH(rpubkey)
	if err != nil {
		return "", fmt.Errorf("ecdh compute secrete err:%v", err.Error())
	}
	c.encoder, _ = rc4.NewCipher(secret[:])
	c.decoder, _ = rc4.NewCipher(secret[:])

	pubBytes := c.pubKey.Bytes()
	pubBase64 := base64.StdEncoding.EncodeToString(pubBytes)

	logger.Info(fmt.Sprintf("remote pubkey:%v", remotePubBase64))
	logger.Info(fmt.Sprintf("pubkey:%v", pubBase64))
	return pubBase64, nil
}

func (c *Crypto) Base64PubKey() string {
	pubBytes := c.pubKey.Bytes()
	pubBase64 := base64.StdEncoding.EncodeToString(pubBytes)
	return pubBase64
}

// 加密
func (c *Crypto) Encode(src []byte, dst []byte) {
	c.encoder.XORKeyStream(src, dst)
}

// 加密
func (c *Crypto) Decode(src []byte, dst []byte) {
	c.decoder.XORKeyStream(dst, src)
}
