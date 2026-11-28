package envfmt

import "testing"

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"plain":        "plain",
		"has space":    `"has space"`,
		`a"b`:          `"a\"b"`,
		"line1\nline2": `"line1\nline2"`,
		`back\slash`:   `"back\\slash"`,
		"x=y":          `"x=y"`,
		"$HOME":        `"$HOME"`,
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestRender(t *testing.T) {
	pairs := [][2]string{{"A", "1"}, {"B", "it's"}, {"my.key", "x"}}
	if got := Render(pairs, "dotenv"); got != "A=1\nB=\"it's\"\nmy.key=x\n" {
		t.Errorf("dotenv: %q", got)
	}
	want := "export A='1'\nexport B='it'\\''s'\n# skipped my.key: not a valid shell variable name\n"
	if got := Render(pairs, "shell"); got != want {
		t.Errorf("shell: %q", got)
	}
}
