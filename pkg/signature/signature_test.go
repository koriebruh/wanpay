//go:build !integration

package signature

import (
	"strings"
	"testing"
)

func TestSign_Verify_SHA256(t *testing.T) {
	key := []byte("secret-key")
	msg := []byte("hello wanpey")

	sig := Sign(key, msg)
	if sig == "" {
		t.Fatal("Sign returned empty string")
	}
	if !Verify(key, msg, sig) {
		t.Error("Verify rejected valid signature")
	}
}

func TestSign_SHA256_Deterministic(t *testing.T) {
	key := []byte("key")
	msg := []byte("msg")
	sig1 := Sign(key, msg)
	sig2 := Sign(key, msg)
	if sig1 != sig2 {
		t.Error("Sign not deterministic — HMAC must be")
	}
}

func TestVerify_SHA256_WrongKey(t *testing.T) {
	sig := Sign([]byte("real-key"), []byte("msg"))
	if Verify([]byte("wrong-key"), []byte("msg"), sig) {
		t.Error("Verify accepted signature with wrong key")
	}
}

func TestVerify_SHA256_WrongMessage(t *testing.T) {
	key := []byte("key")
	sig := Sign(key, []byte("original"))
	if Verify(key, []byte("tampered"), sig) {
		t.Error("Verify accepted signature with wrong message")
	}
}

func TestVerify_SHA256_TamperedSignature(t *testing.T) {
	key := []byte("key")
	msg := []byte("msg")
	sig := Sign(key, msg)
	// Flip one hex digit
	tampered := strings.Replace(sig, sig[:1], "x", 1)
	if Verify(key, msg, tampered) {
		t.Error("Verify accepted tampered signature")
	}
}

func TestVerify_SHA256_EmptySignature(t *testing.T) {
	key := []byte("key")
	msg := []byte("msg")
	if Verify(key, msg, "") {
		t.Error("Verify accepted empty signature")
	}
}

func TestSign_Verify_SHA512(t *testing.T) {
	key := []byte("doku-secret")
	msg := []byte("doku-payload")

	sig := SignSHA512(key, msg)
	if sig == "" {
		t.Fatal("SignSHA512 returned empty string")
	}
	if !VerifySHA512(key, msg, sig) {
		t.Error("VerifySHA512 rejected valid signature")
	}
}

func TestSign_SHA512_Deterministic(t *testing.T) {
	key := []byte("k")
	msg := []byte("m")
	sig1 := SignSHA512(key, msg)
	sig2 := SignSHA512(key, msg)
	if sig1 != sig2 {
		t.Error("SignSHA512 not deterministic")
	}
}

func TestVerify_SHA512_WrongKey(t *testing.T) {
	sig := SignSHA512([]byte("real"), []byte("msg"))
	if VerifySHA512([]byte("fake"), []byte("msg"), sig) {
		t.Error("VerifySHA512 accepted wrong key")
	}
}

func TestVerify_SHA512_TamperedSignature(t *testing.T) {
	key := []byte("k")
	msg := []byte("m")
	sig := SignSHA512(key, msg)
	if VerifySHA512(key, msg, sig+"x") {
		t.Error("VerifySHA512 accepted appended chars")
	}
}

func TestSHA256_SHA512_ProduceDifferentSignatures(t *testing.T) {
	key := []byte("k")
	msg := []byte("m")
	if Sign(key, msg) == SignSHA512(key, msg) {
		t.Error("SHA256 and SHA512 produced identical signatures — unexpected collision")
	}
}
