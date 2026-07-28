package doctype

import (
	"strings"
	"testing"
)

// TestHashPassword_NeverPlaintext is the law: a Password field value is stored
// one-way. The encoded form must be a PHC argon2id string, never the input.
func TestHashPassword_NeverPlaintext(t *testing.T) {
	const pw = "correct horse battery staple"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if strings.Contains(h, pw) {
		t.Fatal("hash contains the plaintext")
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("not a PHC argon2id string: %q", h)
	}
	if !IsHashed(h) {
		t.Fatal("IsHashed does not recognize our own output")
	}
}

// TestHashPassword_Salted: the same plaintext hashed twice must differ, so a
// stolen store yields no equality oracle across rows.
func TestHashPassword_Salted(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("identical plaintexts produced identical hashes — salt is not applied")
	}
	if !VerifyPassword(a, "same") || !VerifyPassword(b, "same") {
		t.Fatal("distinct salted hashes must both verify")
	}
}

func TestVerifyPassword(t *testing.T) {
	h, _ := HashPassword("hunter2")
	if !VerifyPassword(h, "hunter2") {
		t.Fatal("correct password rejected")
	}
	for _, wrong := range []string{"hunter3", "", "HUNTER2", "hunter2 "} {
		if VerifyPassword(h, wrong) {
			t.Fatalf("wrong password %q accepted", wrong)
		}
	}
}

// TestVerifyPassword_MalformedEncodings must fail closed, never panic.
func TestVerifyPassword_Malformed(t *testing.T) {
	for _, enc := range []string{
		"", "not-a-hash", "$argon2id$", "$argon2i$v=19$m=1,t=1,p=1$AAAA$BBBB",
		"$argon2id$v=99$m=1,t=1,p=1$AAAA$BBBB",
		"$argon2id$v=19$garbage$AAAA$BBBB",
		"$argon2id$v=19$m=1,t=1,p=1$!!!!$BBBB",
		"$argon2id$v=19$m=1,t=1,p=1$AAAA$!!!!",
	} {
		if VerifyPassword(enc, "anything") {
			t.Fatalf("malformed encoding %q verified", enc)
		}
	}
}

func TestIsHashed(t *testing.T) {
	if IsHashed("plaintext") || IsHashed("") || IsHashed(RedactedMarker) {
		t.Fatal("IsHashed accepted a non-hash")
	}
}
