package auth

import "testing"

func TestHashVerifyRoundTrip(t *testing.T) {
	encoded, err := Hash("readerss")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !Verify(encoded, "readerss") {
		t.Fatal("expected correct password to verify")
	}
	if Verify(encoded, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
	if NeedsUpgrade(encoded) {
		t.Fatal("freshly hashed password should not need upgrade")
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	a, _ := Hash("same")
	b, _ := Hash("same")
	if a == b {
		t.Fatal("expected different salts to produce different encodings")
	}
}

func TestVerifyLegacySHA256(t *testing.T) {
	// sha256("readress:readerss") base64-url, matching the historical default user.
	const legacy = "sha256:F2dCBytUHPliqCpZHC7mm9-40Kqv-LGB4xZcQWfOSig"
	if !Verify(legacy, "readerss") {
		t.Fatal("expected legacy hash to verify")
	}
	if Verify(legacy, "nope") {
		t.Fatal("expected legacy hash to reject wrong password")
	}
	if !NeedsUpgrade(legacy) {
		t.Fatal("legacy hash should be flagged for upgrade")
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	for _, enc := range []string{"", "plain", "pbkdf2_sha256$bad", "pbkdf2_sha256$10$@@@$@@@"} {
		if Verify(enc, "x") {
			t.Fatalf("expected %q to fail verification", enc)
		}
	}
}
