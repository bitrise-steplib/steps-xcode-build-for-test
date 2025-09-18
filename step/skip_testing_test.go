package step

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXcodebuildBuilder_parseSkipTestingFormat(t *testing.T) {
	tests := []struct {
		name        string
		skipTesting []string
		want        map[string][]TestPathString
		wantErr     string
	}{
		{
			name:        "nil test",
			skipTesting: nil,
			want:        map[string][]TestPathString{},
		},
		{
			name:        "Skipping test target is not supported",
			skipTesting: []string{"BullsEyeUITests"},
			wantErr:     "not yet supported skip testing format: BullsEyeUITests",
		},
		{
			name:        "Skipping test class",
			skipTesting: []string{"BullsEyeUITests/BullsEyeSlowTests"},
			want:        map[string][]TestPathString{"BullsEyeUITests": {"BullsEyeSlowTests"}},
		},
		{
			name:        "Skipping test method",
			skipTesting: []string{"BullsEyeUITests/BullsEyeSlowTests/testExample"},
			want:        map[string][]TestPathString{"BullsEyeUITests": {`BullsEyeSlowTests\/testExample`}},
		},
		{
			name: "Skipping multiple tests",
			skipTesting: []string{
				"BullsEyeUITests/BullsEyeSlowTests/testExample1",
				"BullsEyeUITests/BullsEyeSlowTests/testExample2",
				"BullsEyeUITests/BullsEyeFlakyTests/testExample",
				"BullsEyeUnitTests/BullsEyeFailingTests",
			},
			want: map[string][]TestPathString{
				"BullsEyeUITests": {
					`BullsEyeSlowTests\/testExample1`,
					`BullsEyeSlowTests\/testExample2`,
					`BullsEyeFlakyTests\/testExample`,
				},
				"BullsEyeUnitTests": {
					"BullsEyeFailingTests",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := XcodebuildBuilder{}
			got, err := b.parseSkipTestingFormat(tt.skipTesting)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}
