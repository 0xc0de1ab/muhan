package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDefaultRootIDPrefixKeepsUTF8Base(t *testing.T) {
	got, warning := defaultRootIDPrefix("player/bank/가나다")
	if got != "objinst:가나다" || warning != "" {
		t.Fatalf("prefix=%q warning=%q", got, warning)
	}
}

func TestDefaultRootIDPrefixHashesInvalidUTF8Base(t *testing.T) {
	got, warning := defaultRootIDPrefix("player/bank/" + string([]byte{0xff, 0xfe}))
	if !strings.HasPrefix(got, "objinst:file:") || warning == "" {
		t.Fatalf("prefix=%q warning=%q", got, warning)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("prefix is not valid UTF-8: %q", got)
	}
}

func TestRootLocationUsesRootPrefixForDefaultContainer(t *testing.T) {
	location, warnings, err := rootLocation("objinst:file:abcdef123456", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if location.ContainerID != "objinst:file:abcdef123456:external" {
		t.Fatalf("location = %+v", location)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "synthetic container") {
		t.Fatalf("warnings = %+v", warnings)
	}
}
