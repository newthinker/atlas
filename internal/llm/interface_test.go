package llm

import "testing"

func TestInterfaceDefined(t *testing.T) {
	var _ Provider = nil
}
