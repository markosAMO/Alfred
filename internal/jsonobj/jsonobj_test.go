package jsonobj

import (
	"strings"
	"testing"
)

func TestKeysKeepTheirOrderAcrossARoundTrip(t *testing.T) {
	o, err := Parse([]byte(`{"zeta": 1, "alpha": {"b": 2, "a": 1}, "mid": [3, 1]}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Format(o)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"zeta\": 1,\n  \"alpha\": {\n    \"b\": 2,\n    \"a\": 1\n  },\n  \"mid\": [\n    3,\n    1\n  ]\n}\n"
	if string(out) != want {
		t.Errorf("Format =\n%s\nwant\n%s", out, want)
	}
}

func TestSetReplacesInPlaceAndAppendsNewKeys(t *testing.T) {
	o, err := Parse([]byte(`{"a": 1, "b": 2, "c": 3}`))
	if err != nil {
		t.Fatal(err)
	}
	o.Set("b", []byte(`20`))
	o.Set("d", []byte(`4`))
	o.Delete("a")
	o.Delete("missing")

	if got := strings.Join(o.Keys(), ","); got != "b,c,d" {
		t.Errorf("Keys = %s", got)
	}
	if raw, _ := o.Get("b"); string(raw) != "20" {
		t.Errorf("b = %s", raw)
	}
}

func TestUntouchedValuesKeepTheirBytes(t *testing.T) {
	// A value the installer does not own is not decoded and re-encoded, so its escapes and
	// its non-ASCII text survive exactly.
	o, err := Parse([]byte(`{"k": "a<b ñ \"q\""}`))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := Format(o)
	if !strings.Contains(string(out), `"a<b ñ \"q\""`) {
		t.Errorf("value changed: %s", out)
	}
}

func TestChildOfAMissingOrNonObjectKeyIsEmpty(t *testing.T) {
	o, err := Parse([]byte(`{"list": [1]}`))
	if err != nil {
		t.Fatal(err)
	}
	if o.Child("list").Len() != 0 || o.Child("absent").Len() != 0 {
		t.Error("Child should be empty for a non-object or a missing key")
	}
}

func TestParseRejectsWhatIsNotOneObject(t *testing.T) {
	for _, body := range []string{`[1]`, `"x"`, `{"a": 1} {"b": 2}`, `{"a": }`, ``} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("Parse(%q) should fail", body)
		}
	}
}

func TestRawDoesNotEscapeHTML(t *testing.T) {
	raw, err := Raw(map[string]string{"p": "a < b && c > d"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"p":"a < b && c > d"}` {
		t.Errorf("Raw = %s", raw)
	}
}
