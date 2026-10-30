package noosexit_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/dismoralzor/metalert/cmd/staticlint/noosexit"
)

func TestNoOSExit(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), noosexit.Analyzer, "./...")
}
