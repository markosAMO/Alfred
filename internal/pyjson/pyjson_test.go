package pyjson

import "testing"

func TestEncodeStringMatchesJSONDumps(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, `"plain"`},
		{`quote"and\slash`, `"quote\"and\\slash"`},
		{"tab\tnewline\n", `"tab\tnewline\n"`},
		{"bell\x07", "\"bell\\u0007\""},
		// json.dumps leaves these alone; encoding/json would escape all three.
		{`<tag> & "amp"`, `"<tag> & \"amp\""`},
		// ensure_ascii=True: every rune above U+007E is escaped, astral ones as a pair.
		{"\u00f1and\u00fa", "\"\\u00f1and\\u00fa\""},
		{"emoji \U0001F600", "\"emoji \\ud83d\\ude00\""},
	}

	for _, c := range cases {
		if got := EncodeString(c.in); got != c.want {
			t.Errorf("EncodeString(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestEncodePreservesKeyOrder(t *testing.T) {
	doc := NewObject()
	for _, k := range []string{"zebra", "alpha", "middle"} {
		doc.Set(k, NewString(k))
	}

	want := `{"zebra": "zebra", "alpha": "alpha", "middle": "middle"}`
	if got := Encode(doc, 0); got != want {
		t.Errorf("compact = %s, want %s", got, want)
	}
}

func TestEncodeIndentMatchesPython(t *testing.T) {
	inner := NewObject()
	inner.Set("a", NewBool(true))

	doc := NewObject()
	doc.Set("files", inner)
	doc.Set("empty", NewObject())
	doc.Set("list", NewArray())

	want := "{\n  \"files\": {\n    \"a\": true\n  },\n  \"empty\": {},\n  \"list\": []\n}"
	if got := Encode(doc, 2); got != want {
		t.Errorf("indent =\n%s\nwant\n%s", got, want)
	}
}

func TestSetReplacesInPlaceAndSetDefaultAppends(t *testing.T) {
	doc := NewObject()
	doc.Set("first", NewString("1"))
	doc.Set("second", NewString("2"))
	doc.Set("first", NewString("replaced"))

	if got := Encode(doc, 0); got != `{"first": "replaced", "second": "2"}` {
		t.Errorf("Set did not replace in place: %s", got)
	}

	doc.SetDefault("second", NewString("ignored"))
	doc.SetDefault("third", NewString("3"))
	if got := Encode(doc, 0); got != `{"first": "replaced", "second": "2", "third": "3"}` {
		t.Errorf("SetDefault = %s", got)
	}
}

func TestDecodeRoundTripKeepsOrder(t *testing.T) {
	src := `{"z": 1, "a": {"n": null, "t": true}, "arr": ["x", 2]}`

	doc, err := Decode([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := Encode(doc, 0); got != src {
		t.Errorf("round trip = %s, want %s", got, src)
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	if _, err := Decode([]byte(`{} {}`)); err == nil {
		t.Error("expected an error for trailing data")
	}
}

func TestAccessorsAreNilSafe(t *testing.T) {
	var missing *Value

	if missing.Get("anything") != nil {
		t.Error("Get on nil should be nil")
	}
	if got := missing.StringOr("fallback"); got != "fallback" {
		t.Errorf("StringOr on nil = %q", got)
	}
	if missing.Members() != nil || missing.Keys() != nil || missing.Strings() != nil {
		t.Error("Members/Keys/Strings on nil should be empty")
	}

	// A key that is absent from a real object behaves the same way.
	doc := NewObject()
	if doc.Get("nope").Get("deeper").StringOr("d") != "d" {
		t.Error("chained access through a missing key should fall back")
	}
}

func TestStringsSkipsNonStrings(t *testing.T) {
	doc, err := Decode([]byte(`{"a": ["one", 2, null, "two"]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := doc.Get("a").Strings()
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("Strings() = %v", got)
	}
}
