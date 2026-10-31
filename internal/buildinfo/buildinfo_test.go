package buildinfo

import (
	"bytes"
	"testing"
)

func TestPrint(t *testing.T) {
	tests := []struct {
		name                  string
		version, date, commit string
		want                  string
	}{
		{
			name:    "все значения заданы",
			version: "v1.0.1",
			date:    "2026/10/31 12:00:00",
			commit:  "abc1234",
			want: "Build version: v1.0.1\n" +
				"Build date: 2026/10/31 12:00:00\n" +
				"Build commit: abc1234\n",
		},
		{
			name: "все значения пустые",
			want: "Build version: N/A\n" +
				"Build date: N/A\n" +
				"Build commit: N/A\n",
		},
		{
			name:    "частично пустые",
			version: "v1.0.1",
			want: "Build version: v1.0.1\n" +
				"Build date: N/A\n" +
				"Build commit: N/A\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Print(&buf, tt.version, tt.date, tt.commit)
			if got := buf.String(); got != tt.want {
				t.Errorf("Print():\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestNA(t *testing.T) {
	if got := na(""); got != "N/A" {
		t.Errorf(`na("") = %q, want "N/A"`, got)
	}
	if got := na("x"); got != "x" {
		t.Errorf(`na("x") = %q, want "x"`, got)
	}
}
