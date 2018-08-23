package main

import (
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
)

func TestCanMarshalPatch(t *testing.T) {
	patchBytes, err := generateDeletePatch("foo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jsonpatch.DecodePatch(patchBytes); err != nil {
		t.Fatal(err)
	}
}
