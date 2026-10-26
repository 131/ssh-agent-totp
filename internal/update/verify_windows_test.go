package update

import (
	"os"
	"testing"
)

// SSH_AGENT_TOTP_VERIFY_SIGNED=<path to an Authenticode-signed exe> checks the Windows
// verification against a real signature.
func TestVerifyRelease(t *testing.T) {
	signed := os.Getenv("SSH_AGENT_TOTP_VERIFY_SIGNED")
	if signed == "" {
		t.Skip("SSH_AGENT_TOTP_VERIFY_SIGNED not set")
	}
	if _, name, err := signer(signed); err != nil || name == "" {
		t.Fatalf("signer: %q %v", name, err)
	}
	if err := Authenticode(signed, signed); err != nil {
		t.Fatal(err)
	}
	unsigned, _ := os.Executable()
	if err := Authenticode(signed, unsigned); err == nil {
		t.Fatal("unsigned running executable must refuse updates")
	}
	if err := Authenticode(unsigned, signed); err == nil {
		t.Fatal("unsigned candidate must be refused")
	}
}
