package cueimports

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImport(t *testing.T) {
	err := os.Chdir("testdata")
	require.NoError(t, err)

	tests := []struct {
		filename    string
		passContent bool
		want        string
	}{
		{"file.cue", false, "file.cue.golden"},
		{"file.cue", true, "file.cue.golden"},
		{"default.cue", false, "default.cue.golden"},
		{"default.cue", true, "default.cue.golden"},
		{"unchanged.cue", false, "unchanged.cue.golden"},
		{"unchanged.cue", true, "unchanged.cue.golden"},
		{"unused_import.cue", false, "unused_import.cue.golden"},
		{"unused_import.cue", true, "unused_import.cue.golden"},
		{"top_level_name_clash.cue", false, "top_level_name_clash.cue.golden"},
		{"top_level_name_clash.cue", true, "top_level_name_clash.cue.golden"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%v", tt.filename, tt.passContent), func(t *testing.T) {
			var content []byte
			if tt.passContent {
				content, err = os.ReadFile(tt.filename)
				require.NoError(t, err)
			}
			res, err := Import(tt.filename, content)
			require.NoError(t, err)

			expected, err := os.ReadFile(tt.want)
			require.NoError(t, err)

			require.Equal(t, string(expected), string(res))
		})
	}
}
