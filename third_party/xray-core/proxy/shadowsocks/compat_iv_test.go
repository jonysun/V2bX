package shadowsocks

import (
	"errors"
	"testing"
)

func TestIVReplayCheckCompatibility(t *testing.T) {
	account, err := (&Account{
		Password:   "test-password",
		CipherType: CipherType_AES_128_GCM,
		IvCheck:    true,
	}).AsAccount()
	if err != nil {
		t.Fatal(err)
	}

	memoryAccount := account.(*MemoryAccount)
	iv := []byte("0123456789abcdef")
	if err := memoryAccount.CheckIV(iv); err != nil {
		t.Fatalf("first IV check failed: %v", err)
	}
	if err := memoryAccount.CheckIV(iv); !errors.Is(err, ErrIVNotUnique) {
		t.Fatalf("repeated IV error = %v, want %v", err, ErrIVNotUnique)
	}
	if !memoryAccount.ToProto().(*Account).IvCheck {
		t.Fatal("IV check setting was lost when converting the memory account")
	}
}

func TestIVReplayCheckCanBeDisabled(t *testing.T) {
	account, err := (&Account{
		Password:   "test-password",
		CipherType: CipherType_AES_128_GCM,
	}).AsAccount()
	if err != nil {
		t.Fatal(err)
	}

	iv := []byte("0123456789abcdef")
	for range 2 {
		if err := account.(*MemoryAccount).CheckIV(iv); err != nil {
			t.Fatalf("IV check should be disabled: %v", err)
		}
	}
}
