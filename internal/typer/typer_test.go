package typer

import "testing"

func TestStringToKeyActions_PortugueseAccents(t *testing.T) {
	text := "ação, você!"

	usActions := StringToKeyActions(text, "us-intl")
	// 'a' (1) + 'ç' (2: '+c) + 'ã' (2: ~+a) + 'o' (1) + ',' (1) + ' ' (1) + 'v' (1) + 'o' (1) + 'c' (1) + 'ê' (2: ^+e) + '!' (1) = 14 key actions
	if len(usActions) != 14 {
		t.Fatalf("expected 14 key actions for %q on us-intl, got %d", text, len(usActions))
	}

	abntActions := StringToKeyActions(text, "abnt2")
	// On ABNT2 'ç' is 1 native key (KeySemicolon), so 13 key actions total
	if len(abntActions) != 13 {
		t.Fatalf("expected 13 key actions for %q on abnt2, got %d", text, len(abntActions))
	}
}
