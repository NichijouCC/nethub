package nethub

import (
	"crypto/ecdh"
	"crypto/rand"
	"log"
	"testing"
)

func TestNewCrypto(t *testing.T) {
	privateA, _ := ecdh.P256().GenerateKey(rand.Reader)
	publiceA := privateA.PublicKey()
	bytesA := publiceA.Bytes()
	privateB, _ := ecdh.P256().GenerateKey(rand.Reader)
	publiceB := privateB.PublicKey()
	bytesB := publiceB.Bytes()

	ra, err := ecdh.P256().NewPublicKey(bytesA)
	if err != nil {
		log.Println(err)
	}

	rb, err := ecdh.P256().NewPublicKey(bytesB)
	if err != nil {
		log.Println(err)
	}

	secreteA, _ := privateA.ECDH(rb)
	secreteB, _ := privateB.ECDH(ra)
	log.Println(string(secreteA))
	log.Println(string(secreteB))
}
