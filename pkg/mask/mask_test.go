//go:build !integration

package mask

import (
	"strings"
	"testing"
)

func TestCard_Normal(t *testing.T) {
	got := Card("4111111111111111")
	want := "411111******1111"
	if got != want {
		t.Errorf("Card() = %q, want %q", got, want)
	}
}

func TestCard_WithSpaces(t *testing.T) {
	got := Card("4111 1111 1111 1111")
	want := "411111******1111"
	if got != want {
		t.Errorf("Card(with spaces) = %q, want %q", got, want)
	}
}

func TestCard_ShortPAN(t *testing.T) {
	got := Card("123456789") // < 10 chars
	if got != "**masked**" {
		t.Errorf("Card(short) = %q, want **masked**", got)
	}
}

func TestCard_ExactlyTenChars(t *testing.T) {
	// 10 chars: first 6 + zero middle + last 4
	got := Card("1234567890")
	if got != "123456"+"7890" {
		t.Errorf("Card(10 chars) = %q", got)
	}
}

func TestEmail_Normal(t *testing.T) {
	got := Email("user@example.com")
	if got != "u***@example.com" {
		t.Errorf("Email() = %q, want u***@example.com", got)
	}
}

func TestEmail_SingleCharLocal(t *testing.T) {
	got := Email("u@example.com")
	// single char local: keep it + "***" + domain
	if !strings.HasPrefix(got, "u") || !strings.Contains(got, "@example.com") {
		t.Errorf("Email(single char) = %q unexpected", got)
	}
}

func TestEmail_NoAtSign(t *testing.T) {
	got := Email("notanemail")
	if got != "***@***" {
		t.Errorf("Email(no @) = %q, want ***@***", got)
	}
}

func TestEmail_AtFirst(t *testing.T) {
	// at = 0 means local part is empty → returns ***@***
	got := Email("@example.com")
	if got != "***@***" {
		t.Errorf("Email(@ first) = %q, want ***@***", got)
	}
}

func TestEmail_MultiCharLocal(t *testing.T) {
	got := Email("john@example.com")
	// First char visible, rest masked, domain preserved
	if !strings.HasPrefix(got, "j") {
		t.Errorf("Email(john) first char not preserved: %q", got)
	}
	if !strings.HasSuffix(got, "@example.com") {
		t.Errorf("Email(john) domain not preserved: %q", got)
	}
	if strings.Contains(got, "ohn") {
		t.Errorf("Email(john) local middle exposed: %q", got)
	}
}

func TestPhone_Normal(t *testing.T) {
	got := Phone("081234567890")
	// 12 digits: first 4 visible, last 4 visible, middle masked
	if !strings.HasPrefix(got, "0812") {
		t.Errorf("Phone prefix wrong: %q", got)
	}
	if !strings.HasSuffix(got, "7890") {
		t.Errorf("Phone suffix wrong: %q", got)
	}
	if strings.Contains(got, "3456") {
		t.Errorf("Phone middle digits exposed: %q", got)
	}
}

func TestPhone_WithPlus(t *testing.T) {
	got := Phone("+6281234567890")
	if !strings.HasPrefix(got, "+628") {
		t.Errorf("Phone(+62) prefix wrong: %q", got)
	}
}

func TestPhone_TooShort(t *testing.T) {
	got := Phone("1234567") // 7 digits < 8
	if got != "****" {
		t.Errorf("Phone(short) = %q, want ****", got)
	}
}

func TestPhone_WithDashes(t *testing.T) {
	// Dashes stripped — only digits kept
	got := Phone("0812-3456-7890")
	if !strings.HasPrefix(got, "0812") {
		t.Errorf("Phone(dashes) prefix wrong: %q", got)
	}
}

func TestName_SingleWord(t *testing.T) {
	got := Name("Alice")
	if !strings.HasPrefix(got, "A") {
		t.Errorf("Name(single) first char wrong: %q", got)
	}
	if strings.Contains(got, "lice") {
		t.Errorf("Name(single) rest exposed: %q", got)
	}
}

func TestName_MultiWord(t *testing.T) {
	got := Name("John Doe")
	parts := strings.Fields(got)
	if len(parts) != 2 {
		t.Fatalf("Name(multi) word count = %d, want 2", len(parts))
	}
	if !strings.HasPrefix(parts[0], "J") || strings.Contains(parts[0], "ohn") {
		t.Errorf("Name first word wrong: %q", parts[0])
	}
	if !strings.HasPrefix(parts[1], "D") || strings.Contains(parts[1], "oe") {
		t.Errorf("Name second word wrong: %q", parts[1])
	}
}

func TestName_SingleCharWord(t *testing.T) {
	got := Name("J D")
	// Single-char words left as-is (no asterisks to add)
	parts := strings.Fields(got)
	if len(parts) != 2 || parts[0] != "J" || parts[1] != "D" {
		t.Errorf("Name(single chars) = %q, want J D", got)
	}
}

func TestName_Empty(t *testing.T) {
	got := Name("")
	if got != "" {
		t.Errorf("Name(empty) = %q, want empty", got)
	}
}

func TestSecret_AlwaysRedacted(t *testing.T) {
	cases := []string{"wpay_live_abc123", "", "super-secret", "sk_test_xyz"}
	for _, s := range cases {
		if got := Secret(s); got != "[REDACTED]" {
			t.Errorf("Secret(%q) = %q, want [REDACTED]", s, got)
		}
	}
}
