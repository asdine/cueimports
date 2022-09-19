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

func TestImportInsertPosition(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty file",
			input: "",
			want:  "\n",
		},
		{
			name:  "no package",
			input: "b: math.Round(1.5)",
			want: `import "math"

b: math.Round(1.5)
`,
		},
		{
			name: "with package",
			input: `package test

			b: math.Round(1.5)
`,
			want: `package test

import "math"

b: math.Round(1.5)
`,
		},
		{
			name: "with comment",
			input: `package test
			// some comment

			b: math.Round(1.5)
`,
			want: `package test

// some comment
import "math"

b: math.Round(1.5)
`,
		},
		{
			name: "with comment at the top",
			input: `// some comment
			package test

			b: math.Round(1.5)
`,
			want: `// some comment
package test

import "math"

b: math.Round(1.5)
`,
		},
		{
			name: "with existing import",
			input: `import "math"

b: math.Round(1.5)
`,
			want: `import "math"

b: math.Round(1.5)
`,
		},
		{
			name: "with existing different import",
			input: `import "encoding/json"

b: math.Round(1.5)
c: json.Marshal(1)
`,
			want: `import (
	"encoding/json"
	"math"
)

b: math.Round(1.5)
c: json.Marshal(1)
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Import("", []byte(tt.input))
			require.NoError(t, err)
			require.Equal(t, tt.want, string(res))
		})
	}
}
