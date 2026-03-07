package analysis

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
)

func TestCallgraph(t *testing.T) {
	prog, _, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)
	cg := BuildCallGraph(prog)
	printCallgraph(cg, prog)
}
