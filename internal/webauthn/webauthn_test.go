package webauthn

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"evilion.xyz/chat/internal/webauthn/fake"
)

func clientData(typ string, challenge []byte, origin string) []byte {
	b, _ := json.Marshal(ClientData{Type: typ, Challenge: base64.RawURLEncoding.EncodeToString(challenge), Origin: origin})
	return b
}

func TestRegisterThenSignIn(t *testing.T) {
	for _, alg := range []int64{AlgES256, AlgEdDSA} {
		a := fake.New(alg)
		challenge := []byte("register-challenge-0123456789abc")
		att := a.Create("nexa.pm")
		authData, err := AuthDataFromAttestation(att)
		if err != nil {
			t.Fatal(err)
		}
		ad, err := ParseAuthData(authData, "nexa.pm")
		if err != nil {
			t.Fatalf("alg %d: %v", alg, err)
		}
		if !SameCredential(ad.CredentialID, a.ID) || ad.Alg != alg {
			t.Fatalf("alg %d: credential not read back", alg)
		}
		spki, err := MarshalKey(ad.PublicKey)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseAuthData(authData, "evil.example"); err == nil {
			t.Fatal("a passkey made for another site was taken")
		}
		cd := clientData("webauthn.get", challenge, "https://nexa.pm")
		if o, err := CheckClientData(cd, "webauthn.get", challenge); err != nil || o != "https://nexa.pm" {
			t.Fatalf("client data: %v %q", err, o)
		}
		if _, err := CheckClientData(cd, "webauthn.get", []byte("another")); err == nil {
			t.Fatal("another challenge was taken")
		}
		if _, err := CheckClientData(cd, "webauthn.create", challenge); err == nil {
			t.Fatal("the wrong type was taken")
		}
		gotAD, sig := a.Get("nexa.pm", cd)
		if err := VerifySignature(spki, alg, gotAD, cd, sig); err != nil {
			t.Fatalf("alg %d: good signature refused: %v", alg, err)
		}
		sig[len(sig)-1] ^= 1
		if VerifySignature(spki, alg, gotAD, cd, sig) == nil {
			t.Fatalf("alg %d: a broken signature was taken", alg)
		}
	}
}

func TestUserVerificationIsRequired(t *testing.T) {
	h := sha256.Sum256([]byte("nexa.pm"))
	raw := append(h[:], flagUP, 0, 0, 0, 1)
	if _, err := ParseAuthData(raw, "nexa.pm"); err == nil {
		t.Fatal("an answer without user verification was taken")
	}
	raw[32] = flagUP | flagUV
	if _, err := ParseAuthData(raw, "nexa.pm"); err != nil {
		t.Fatal(err)
	}
}

func TestBadCOSEKeysAreRefused(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pt, _ := k.PublicKey.Bytes()
	good := map[any]any{int64(1): int64(2), int64(3): int64(AlgES256), int64(-1): int64(1), int64(-2): pt[1:33], int64(-3): pt[33:]}
	if _, _, err := coseKey(good); err != nil {
		t.Fatal(err)
	}
	off := map[any]any{int64(1): int64(2), int64(3): int64(AlgES256), int64(-1): int64(1), int64(-2): pt[1:33], int64(-3): make([]byte, 32)}
	if _, _, err := coseKey(off); err == nil {
		t.Fatal("a point off the curve was taken")
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, _, err := coseKey(map[any]any{int64(1): int64(1), int64(3): int64(AlgES256), int64(-1): int64(6), int64(-2): []byte(pub)}); err == nil {
		t.Fatal("a key and algorithm that do not belong together were taken")
	}
}

func TestCBORRefusesWhatItCannotRead(t *testing.T) {
	for _, b := range [][]byte{{}, {0x5f}, {0x9f}, {0xa1, 0x41, 0x00, 0x00}, {0x5a, 0xff, 0xff, 0xff, 0xff}} {
		if _, _, err := decodeCBOR(b, 0); err == nil {
			t.Fatalf("% x was read", b)
		}
	}
	var deep []byte
	for i := 0; i < 20; i++ {
		deep = append(deep, 0x81)
	}
	deep = append(deep, 0)
	if _, _, err := decodeCBOR(deep, 0); err == nil {
		t.Fatal("nesting without end was read")
	}
}
