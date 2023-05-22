package nethub

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rc4"
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
func (c *Crypto) ComputeSecret(remotePubKey []byte) ([]byte, error) {
	rpubkey, err := c.priKey.Curve().NewPublicKey(remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh compute secrete err:%v", err.Error())
	}
	secret, err := c.priKey.ECDH(rpubkey)
	if err != nil {
		return nil, fmt.Errorf("ecdh compute secrete err:%v", err.Error())
	}
	c.encoder, _ = rc4.NewCipher(secret)
	c.decoder, _ = rc4.NewCipher(secret)
	return c.pubKey.Bytes(), nil
}

// 加密
func (c *Crypto) Encode(src []byte, dst []byte) {
	c.encoder.XORKeyStream(src, src)
}

// 加密
func (c *Crypto) Decode(src []byte, dst []byte) {
	c.decoder.XORKeyStream(dst, src)
}
