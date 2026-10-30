package handler

import "testing"

func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "plain gzip", header: "gzip", want: true},
		{name: "gzip among several", header: "deflate, gzip", want: true},
		{name: "gzip disabled by q=0", header: "gzip;q=0", want: false},
		{name: "gzip with non-zero q", header: "gzip;q=0.5", want: true},
		{name: "other encoding", header: "br", want: false},
		{name: "empty header", header: "", want: false},
		{name: "case-insensitive", header: "GZIP", want: true},
		{name: "substring must not match", header: "x-gzip", want: false},
		{name: "deflate only", header: "deflate", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acceptsGzip(tt.header); got != tt.want {
				t.Errorf("acceptsGzip(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}
