package cueimports

import (
	"os"
	"testing"
)

func TestImport(t *testing.T) {
	content, err := os.ReadFile("testdata/file.cue")
	if err != nil {
		t.Fatalf("failed to read testdata/file.cue: %v", err)
	}

	res, err := Import(StdinFilename, content)
	if err != nil {
		t.Fatalf("failed to import: %v", err)
	}

	expected, err := os.ReadFile("testdata/file.cue.golden")
	if err != nil {
		t.Fatalf("failed to read testdata/file.cue.golden: %v", err)
	}

	if string(res) != string(expected) {
		t.Fatalf("expected %s, got %s", expected, res)
	}
}
