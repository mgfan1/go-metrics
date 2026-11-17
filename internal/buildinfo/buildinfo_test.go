package buildinfo

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrint(t *testing.T) {
	tests := []struct {
		name    string
		version string
		date    string
		commit  string
		want    string
	}{
		{
			name: "сборка без ldflags",
			want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:    "все значения заданы",
			version: "v1.7.0",
			date:    "2026-11-18 12:00:00",
			commit:  "2c4ba53",
			want:    "Build version: v1.7.0\nBuild date: 2026-11-18 12:00:00\nBuild commit: 2c4ba53\n",
		},
		{
			name:    "задана только версия",
			version: "v1.7.0",
			want:    "Build version: v1.7.0\nBuild date: N/A\nBuild commit: N/A\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			Print(&out, tt.version, tt.date, tt.commit)

			assert.Equal(t, tt.want, out.String())
		})
	}
}
