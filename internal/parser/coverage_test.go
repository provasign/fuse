package parser

import (
	"testing"

	"github.com/provasign/fuse/internal/core"
)

func TestSupported(t *testing.T) {
	if !Supported(core.LangGo) {
		t.Error("go should be supported")
	}
	if !Supported(core.LangJSON) {
		t.Error("json should be supported")
	}
	if Supported(core.LangUnknown) {
		t.Error("unknown should not be supported")
	}
}
