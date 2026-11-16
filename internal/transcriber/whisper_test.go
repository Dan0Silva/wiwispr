package transcriber

import "testing"

func TestCleanWhisperOutput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "Normal Portuguese sentence",
			raw:  " Olá, tudo bem? Isso é um teste de transcrição!\n",
			want: "Olá, tudo bem? Isso é um teste de transcrição!",
		},
		{
			name: "Strips bracketed artifacts and keeps speech",
			raw:  "[Música] Testando microfone no Linux (risos)\n",
			want: "Testando microfone no Linux",
		},
		{
			name: "Filters known silence hallucinations",
			raw:  "Legendas pela comunidade Amara.org\n[Silêncio]\n",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanWhisperOutput(tc.raw)
			if got != tc.want {
				t.Errorf("CleanWhisperOutput(%q) = %q; want %q", tc.raw, got, tc.want)
			}
		})
	}
}
