package nethub

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha256"
	"github.com/pkg/errors"
)

// ecdh+rc4
type Crypto struct {
	priKey  *ecdh.PrivateKey
	pubKey  *ecdh.PublicKey
	encoder *rc4.Cipher
	decoder *rc4.Cipher
	//0还未交换secret 1已交换secret 2使用secret
	state int

	priv *ecdsa.PrivateKey
}

func NewCrypto() *Crypto {
	priKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	pubKey := priKey.PublicKey()

	p, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(errors.Wrap(err, "Could not generate private key"))
	}
	return &Crypto{priKey: priKey, pubKey: pubKey, priv: p}
}

// 与不安全的远程协商通信secret，返回pubkey
func (c *Crypto) ComputeSecret(remotePubKey []byte) ([]byte, error) {
	x, y := elliptic.Unmarshal(elliptic.P256(), remotePubKey)
	pub := c.priv.PublicKey
	data := elliptic.Marshal(pub, pub.X, pub.Y)

	seBytes, _ := pub.Curve.ScalarMult(x, y, c.priv.D.Bytes())
	secret := sha256.Sum256(seBytes.Bytes())

	//rpubkey, err := ecdh.P256().NewPublicKey(remotePubKey)
	//if err != nil {
	//	return nil, fmt.Errorf("ecdh compute secrete err:%v", err.Error())
	//}
	//secret, err := c.priKey.ECDH(rpubkey)
	//if err != nil {
	//	return nil, fmt.Errorf("ecdh compute secrete err:%v", err.Error())
	//}
	c.encoder, _ = rc4.NewCipher(secret[:])
	c.decoder, _ = rc4.NewCipher(secret[:])

	return data, nil
}

// 加密
func (c *Crypto) Encode(src []byte, dst []byte) {
	c.encoder.XORKeyStream(src, dst)
}

// 加密
func (c *Crypto) Decode(src []byte, dst []byte) {
	c.decoder.XORKeyStream(dst, src)
}
