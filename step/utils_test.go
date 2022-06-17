package step

import "testing"

func Test_findBuildSetting(t *testing.T) {
	tests := []struct {
		name    string
		options []string
		key     string
		want    string
	}{
		{
			name:    "nil test",
			options: nil,
			key:     "SYMROOT",
			want:    "",
		},
		{
			name:    "empty test",
			options: []string{""},
			key:     "SYMROOT",
			want:    "",
		},
		{
			name:    "SYMROOT not found",
			options: []string{"-resultBundlePath", "tmp", "ARCHS=arm64"},
			key:     "SYMROOT",
			want:    "",
		},
		{
			name:    "SYMROOT found",
			options: []string{"-resultBundlePath", "tmp", "SYMROOT=tmp", "ARCHS=arm64"},
			key:     "SYMROOT",
			want:    "tmp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findBuildSetting(tt.options, tt.key); got != tt.want {
				t.Errorf("findBuildSetting() = %v, want %v", got, tt.want)
			}
		})
	}
}
