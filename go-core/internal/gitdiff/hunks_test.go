package gitdiff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffTextToHunks_DetectsChangedMiddleSection(t *testing.T) {
	old := "line1\nline2\nline3\nline4\nline5\n"
	new := "line1\nline2\nCHANGED\nline4\nline5\n"

	hunks := diffTextToHunks(old, new)

	require_len := 1
	assert.Len(t, hunks, require_len)
	assert.Equal(t, 3, hunks[0].StartLine)
	assert.Equal(t, 3, hunks[0].EndLine)
}

func TestDiffTextToHunks_NoChangeReturnsNil(t *testing.T) {
	same := "line1\nline2\n"
	assert.Nil(t, diffTextToHunks(same, same))
}

func TestDiffTextToHunks_SignatureLineChangeIsDetectedAtCorrectLine(t *testing.T) {
	old := "def apply_discount(self, amount, rate):\n    return amount * rate\n"
	new := "def apply_discount(self, amount, rate, currency):\n    return amount * rate\n"

	hunks := diffTextToHunks(old, new)

	require_len := 1
	assert.Len(t, hunks, require_len)
	assert.Equal(t, 1, hunks[0].StartLine, "the signature line itself must be flagged as changed")
}

func TestMergeAdjacentHunks_CoalescesOverlapping(t *testing.T) {
	merged := mergeAdjacentHunks([]Hunk{{5, 5}, {6, 8}, {20, 25}})
	assert.Equal(t, []Hunk{{5, 8}, {20, 25}}, merged)
}
