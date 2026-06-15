package verify

import (
	"github.com/cloudflare/circl/sign"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// pqScheme is ML-DSA-65 (FIPS 204), NIST security level 3.
var pqScheme sign.Scheme = mldsa65.Scheme()

// GenPQKeyPair returns a fresh ML-DSA-65 keypair as raw scheme bytes.
func GenPQKeyPair() (pub, priv []byte, err error) {
	pk, sk, err := pqScheme.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	pub, err = pk.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	priv, err = sk.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	return pub, priv, nil
}

// SignPQ signs data with a raw ML-DSA-65 private key.
func SignPQ(privRaw, data []byte) ([]byte, error) {
	sk, err := pqScheme.UnmarshalBinaryPrivateKey(privRaw)
	if err != nil {
		return nil, err
	}
	return pqScheme.Sign(sk, data, nil), nil
}

// VerifyPQ reports whether sig is a valid ML-DSA-65 signature of data under pubRaw.
func VerifyPQ(pubRaw, data, sig []byte) (bool, error) {
	pk, err := pqScheme.UnmarshalBinaryPublicKey(pubRaw)
	if err != nil {
		return false, err
	}
	return pqScheme.Verify(pk, data, sig, nil), nil
}
