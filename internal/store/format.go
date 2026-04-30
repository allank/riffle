package store

var Magic = [4]byte{0x53, 0x45, 0x4D, 0x41} // "SEMA"

const Version = uint16(1)

type Header struct {
	Magic     [4]byte
	Version   uint16
	Flags     uint16
	RootHash  [32]byte
	DirCount  uint32
	BuildTime int64
}

type NodeEntry struct {
	PathHash   [32]byte
	MerkleHash [32]byte
	PathOffset uint32
	VectorID   uint32
	MTime      int64
}
