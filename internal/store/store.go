package store

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/allank/riffle/internal/vector"
)

type Node struct {
	RelPath    string
	MerkleHash [32]byte
	VectorID   uint32
	MTime      int64
}

type Store struct {
	Header  Header
	ExtList []string
	Nodes   []Node
	Vector  vector.Index
	root    string
}

func New(root string, exts []string) *Store {
	return &Store{
		Header: Header{
			Magic:   Magic,
			Version: Version,
		},
		ExtList: exts,
		root:    root,
	}
}

func (s *Store) AddNode(n Node) {
	s.Nodes = append(s.Nodes, n)
	s.Header.DirCount = uint32(len(s.Nodes))
}

func (s *Store) SetRootHash(h [32]byte) { s.Header.RootHash = h }

func (s *Store) NodeByPath(relPath string) (Node, bool) {
	ph := sha256.Sum256([]byte(relPath))
	for _, n := range s.Nodes {
		if sha256.Sum256([]byte(n.RelPath)) == ph {
			return n, true
		}
	}
	return Node{}, false
}

// Save writes the store atomically to path (writes to path+".tmp", then renames).
func (s *Store) Save(path string) error {
	s.Header.BuildTime = time.Now().Unix()
	s.Header.DirCount = uint32(len(s.Nodes))

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { os.Remove(tmp) }()

	if err := s.writeTo(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) writeTo(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, s.Header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, uint8(len(s.ExtList))); err != nil {
		return err
	}
	for _, e := range s.ExtList {
		b := []byte(e)
		if err := binary.Write(w, binary.LittleEndian, uint16(len(b))); err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	for _, n := range s.Nodes {
		ph := sha256.Sum256([]byte(n.RelPath))
		entry := NodeEntry{
			PathHash:   ph,
			MerkleHash: n.MerkleHash,
			VectorID:   n.VectorID,
			MTime:      n.MTime,
			PathOffset: uint32(len(n.RelPath)),
		}
		if err := binary.Write(w, binary.LittleEndian, entry); err != nil {
			return fmt.Errorf("write node: %w", err)
		}
		if _, err := w.Write([]byte(n.RelPath)); err != nil {
			return err
		}
	}
	if s.Vector != nil {
		return s.Vector.Save(w)
	}
	return nil
}

// Open reads an index file. Pass a pre-constructed vector.Index to load into.
func Open(path string, idx vector.Index) (*Store, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readFrom(f, idx)
}

func readFrom(r io.Reader, idx vector.Index) (*Store, error) {
	s := &Store{}
	if err := binary.Read(r, binary.LittleEndian, &s.Header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if s.Header.Magic != Magic {
		return nil, fmt.Errorf("invalid magic bytes")
	}
	var extCount uint8
	if err := binary.Read(r, binary.LittleEndian, &extCount); err != nil {
		return nil, err
	}
	s.ExtList = make([]string, extCount)
	for i := range s.ExtList {
		var l uint16
		if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
			return nil, err
		}
		buf := make([]byte, l)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		s.ExtList[i] = string(buf)
	}
	s.Nodes = make([]Node, s.Header.DirCount)
	for i := range s.Nodes {
		var entry NodeEntry
		if err := binary.Read(r, binary.LittleEndian, &entry); err != nil {
			return nil, fmt.Errorf("read node %d: %w", i, err)
		}
		pathBytes := make([]byte, entry.PathOffset)
		if _, err := io.ReadFull(r, pathBytes); err != nil {
			return nil, err
		}
		s.Nodes[i] = Node{
			RelPath:    string(pathBytes),
			MerkleHash: entry.MerkleHash,
			VectorID:   entry.VectorID,
			MTime:      entry.MTime,
		}
	}
	if idx != nil {
		if err := idx.Load(r); err != nil {
			return nil, fmt.Errorf("load vector index: %w", err)
		}
		s.Vector = idx
	}
	return s, nil
}
