package subtree

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

// hostileParentCount builds a TxInpoints body whose declared parent count is
// `count` but which carries no parents at all — the shape a corrupt or hostile
// length prefix produces.
func hostileParentCount(count uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, count)

	return b
}

// TestDeserializeDoesNotAllocateFromAnUntrustedCount pins that the declared
// parent count cannot drive the allocation. Before this was bounded, a count of
// 0xFFFFFFFF asked for 4294967295 chainhash.Hash values — about 137 GB — and the
// process was OOM-killed before reading a single parent. Because the count is
// read from a cache file on disk, that repeated on every restart.
//
// The assertion is on bytes actually allocated, not merely on the error: a test
// that only checked for an error would pass just as well against code that
// allocated 137 GB first and failed afterwards.
func TestDeserializeDoesNotAllocateFromAnUntrustedCount(t *testing.T) {
	for _, count := range []uint32{0xFFFFFFFF, 0x80000000, 0x40000000, 1 << 20} {
		var before, after runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&before)

		_, err := NewTxInpointsFromBytes(hostileParentCount(count))
		require.Error(t, err, "a count with no body behind it must fail")

		runtime.ReadMemStats(&after)

		grew := after.TotalAlloc - before.TotalAlloc

		// Generous ceiling: the point is the difference between kilobytes and
		// gigabytes, not a precise budget.
		require.Less(t, grew, uint64(8<<20),
			"declared count %d allocated %d bytes; it must not be sized from the wire", count, grew)
	}
}

// TestDeserializeStillHandlesCountsAboveThePreallocLimit pins that the bound is
// a buffer-sizing hint and not a protocol limit: a genuine count larger than the
// limit must still round-trip exactly.
func TestDeserializeStillHandlesCountsAboveThePreallocLimit(t *testing.T) {
	const parents = inpointsPreallocLimit + 17

	var p TxInpoints

	for i := 0; i < parents; i++ {
		h := chainhash.HashH([]byte{byte(i), byte(i >> 8), 0xab})
		p.appendInput(h, uint32(i%3))
	}

	require.Len(t, p.ParentTxHashes, parents)

	serialized, err := p.Serialize()
	require.NoError(t, err)

	got, err := NewTxInpointsFromBytes(serialized)
	require.NoError(t, err)
	require.Equal(t, p.ParentTxHashes, got.ParentTxHashes)
	require.Equal(t, p.voutIdxs, got.voutIdxs)
}
