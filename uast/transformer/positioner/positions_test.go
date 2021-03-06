package positioner

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/bblfsh/sdk.v2/uast"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes"
)

func offset(v int) nodes.Object {
	return uast.Position{Offset: uint32(v)}.ToObject()
}

func lineCol(line, col int) nodes.Object {
	return uast.Position{Line: uint32(line), Col: uint32(col)}.ToObject()
}

func fullPos(off, line, col int) nodes.Object {
	return uast.Position{Offset: uint32(off), Line: uint32(line), Col: uint32(col)}.ToObject()
}

func TestFillLineColNested(t *testing.T) {
	require := require.New(t)

	data := "hello\n\nworld"

	input := nodes.Object{
		"a": nodes.Object{
			uast.KeyStart: offset(0),
			uast.KeyEnd:   offset(4),
		},
		"b": nodes.Array{nodes.Object{
			uast.KeyStart: offset(7),
			uast.KeyEnd:   offset(12),
		}},
	}

	expected := nodes.Object{
		"a": nodes.Object{
			uast.KeyStart: fullPos(0, 1, 1),
			uast.KeyEnd:   fullPos(4, 1, 5),
		},
		"b": nodes.Array{nodes.Object{
			uast.KeyStart: fullPos(7, 3, 1),
			uast.KeyEnd:   fullPos(12, 3, 6),
		}},
	}

	p := FromOffset()
	out, err := p.OnCode(data).Do(input)
	require.NoError(err)
	require.Equal(expected, out)
}

func TestFillOffsetNested(t *testing.T) {
	require := require.New(t)

	data := "hello\n\nworld"

	input := nodes.Object{
		"a": nodes.Object{
			uast.KeyStart: lineCol(1, 1),
			uast.KeyEnd:   lineCol(1, 5),
		},
		"b": nodes.Array{nodes.Object{
			uast.KeyStart: lineCol(3, 1),
			uast.KeyEnd:   lineCol(3, 6),
		}},
	}

	expected := nodes.Object{
		"a": nodes.Object{
			uast.KeyStart: fullPos(0, 1, 1),
			uast.KeyEnd:   fullPos(4, 1, 5),
		},
		"b": nodes.Array{nodes.Object{
			uast.KeyStart: fullPos(7, 3, 1),
			uast.KeyEnd:   fullPos(12, 3, 6),
		}},
	}

	p := FromLineCol()
	out, err := p.OnCode(data).Do(input)
	require.NoError(err)
	require.Equal(expected, out)
}

func TestFillOffsetEmptyFile(t *testing.T) {
	require := require.New(t)

	data := ""

	input := nodes.Object{
		uast.KeyStart: lineCol(1, 1),
		uast.KeyEnd:   lineCol(1, 1),
	}

	expected := nodes.Object{
		uast.KeyStart: fullPos(0, 1, 1),
		uast.KeyEnd:   fullPos(0, 1, 1),
	}

	p := FromLineCol()
	out, err := p.OnCode(data).Do(input)
	require.NoError(err)
	require.Equal(expected, out)
}

func TestPosIndex(t *testing.T) {
	// Verify that a multi-byte Unicode rune does not displace offsets after
	// its occurrence in the input. Test few other simple cases as well.
	const source = `line1
ё2
a3`
	var cases = []uast.Position{
		{Offset: 0, Line: 1, Col: 1},
		{Offset: 4, Line: 1, Col: 5},

		// multi-byte unicode rune
		{Offset: 6, Line: 2, Col: 1},
		{Offset: 8, Line: 2, Col: 3}, // col is a byte offset+1, not a rune index

		{Offset: 10, Line: 3, Col: 1},
		{Offset: 11, Line: 3, Col: 2},

		// special case — EOF position
		{Offset: 12, Line: 3, Col: 3},
	}

	ind := newPositionIndex([]byte(source))
	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			line, col, err := ind.LineCol(int(c.Offset))
			require.NoError(t, err)
			require.Equal(t, c.Line, uint32(line))
			require.Equal(t, c.Col, uint32(col))

			off, err := ind.Offset(int(c.Line), int(c.Col))
			require.NoError(t, err)
			require.Equal(t, c.Offset, uint32(off))
		})
	}
}

func TestPosIndexUnicode(t *testing.T) {
	// Verify that a rune offset -> byte offset conversion works.
	const source = `line1
ё2
a3`
	var cases = []struct {
		runeOff   int
		byteOff   int
		line, col int
	}{
		{runeOff: 0, byteOff: 0, line: 1, col: 1},

		// multi-byte unicode rune
		{runeOff: 6, byteOff: 6, line: 2, col: 1},

		{runeOff: 7, byteOff: 8, line: 2, col: 3},
		{runeOff: 10, byteOff: 11, line: 3, col: 2},

		// special case — EOF position
		{runeOff: 11, byteOff: 12, line: 3, col: 3},
	}

	ind := newPositionIndexUnicode([]byte(source))
	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			off, err := ind.RuneOffset(c.runeOff)
			require.NoError(t, err)
			require.Equal(t, c.byteOff, off)

			// verify that offset -> line/col conversion still works
			line, col, err := ind.LineCol(off)
			require.NoError(t, err)
			require.Equal(t, c.line, line)
			require.Equal(t, c.col, col)
		})
	}
}
