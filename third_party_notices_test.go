package main

import (
	"strings"
	"testing"
)

func TestGetThirdPartyNoticesReturnsEmbeddedContent(t *testing.T) {
	got := NewApp().GetThirdPartyNotices()
	if strings.TrimSpace(got) == "" {
		t.Fatal("GetThirdPartyNotices returned an empty notice")
	}
	if got != thirdPartyNotices {
		t.Fatal("GetThirdPartyNotices did not return the embedded notice verbatim")
	}
}
