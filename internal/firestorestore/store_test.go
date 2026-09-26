package firestorestore

import "testing"

func TestHashIsStableAndDoesNotExposeInput(t *testing.T) {
	first := hash("User@Example.com")
	second := hash("User@Example.com")
	if first != second {
		t.Fatal("hash must be stable")
	}
	if first == "User@Example.com" || len(first) != 64 {
		t.Fatalf("unexpected hash %q", first)
	}
}

func TestBuildContactSearchTokensNormalizesPhoneAndNamePrefixes(t *testing.T) {
	contact := Contact{PhoneNumber: "+90 (555) 123-45-67", ProfileName: "İpek Enerji"}
	tokens := BuildContactSearchTokens(contact)
	wanted := map[string]bool{"90555": false, "ipek": false, "ener": false}
	for _, token := range tokens {
		if _, ok := wanted[token]; ok {
			wanted[token] = true
		}
	}
	for token, found := range wanted {
		if !found {
			t.Fatalf("missing search token %q in %v", token, tokens)
		}
	}
	contact.IsDeleted = true
	if deletedTokens := BuildContactSearchTokens(contact); len(deletedTokens) != 0 {
		t.Fatalf("deleted contact should not be searchable: %v", deletedTokens)
	}
}

func TestNewWithClientRejectsNil(t *testing.T) {
	if _, err := NewWithClient(nil, "test"); err == nil {
		t.Fatal("expected nil client to be rejected")
	}
}
