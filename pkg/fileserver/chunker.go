package fileserver

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sync"

	"isthmus/internal/logger"
)

const DefaultBlockSize = 1024 * 1024 // 1MB blocks

type BlockInfo struct {
	Index    int    `json:"index"`
	Offset   int64  `json:"offset"`
	Size     int    `json:"size"`
	Checksum string `json:"checksum"`
}

type FileManifest struct {
	Filename     string      `json:"filename"`
	TotalSize    int64       `json:"total_size"`
	BlockSize    int         `json:"block_size"`
	TotalBlocks  int         `json:"total_blocks"`
	Blocks       []BlockInfo `json:"blocks"`
	FileChecksum string      `json:"file_checksum"`
}

type Chunker struct {
	blockSize int
	log       *logger.Logger
}

func NewChunker(blockSize int) *Chunker {
	if blockSize <= 0 {
		blockSize = DefaultBlockSize
	}
	return &Chunker{
		blockSize: blockSize,
		log:       logger.WithPrefix("Chunker"),
	}
}

// GenerateManifest slices a local file into content-addressed verified blocks
func (c *Chunker) GenerateManifest(filePath string) (*FileManifest, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	totalSize := fi.Size()
	totalBlocks := int((totalSize + int64(c.blockSize) - 1) / int64(c.blockSize))
	if totalBlocks == 0 {
		totalBlocks = 1
	}

	manifest := &FileManifest{
		Filename:    fi.Name(),
		TotalSize:   totalSize,
		BlockSize:   c.blockSize,
		TotalBlocks: totalBlocks,
		Blocks:      make([]BlockInfo, 0, totalBlocks),
	}

	fullHasher := sha256.New()
	buf := make([]byte, c.blockSize)
	var offset int64 = 0

	for idx := 0; idx < totalBlocks; idx++ {
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		if n == 0 {
			break
		}

		fullHasher.Write(buf[:n])

		blockHasher := sha256.New()
		blockHasher.Write(buf[:n])
		blockChecksum := hex.EncodeToString(blockHasher.Sum(nil))

		manifest.Blocks = append(manifest.Blocks, BlockInfo{
			Index:    idx,
			Offset:   offset,
			Size:     n,
			Checksum: blockChecksum,
		})

		offset += int64(n)
	}

	manifest.FileChecksum = hex.EncodeToString(fullHasher.Sum(nil))
	return manifest, nil
}

// VerifyBlock validates an in-memory block against its expected checksum
func (c *Chunker) VerifyBlock(data []byte, expectedChecksum string) bool {
	h := sha256.New()
	h.Write(data)
	calculated := hex.EncodeToString(h.Sum(nil))
	return calculated == expectedChecksum
}

// AssembleChunks writes verified blocks into target destination file
type BlockWriter struct {
	file *os.File
	mu   sync.Mutex
}

func NewBlockWriter(destPath string, totalSize int64) (*BlockWriter, error) {
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(totalSize); err != nil {
		f.Close()
		return nil, err
	}
	return &BlockWriter{file: f}, nil
}

func (bw *BlockWriter) WriteBlock(offset int64, data []byte) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	_, err := bw.file.WriteAt(data, offset)
	return err
}

func (bw *BlockWriter) Close() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.file.Close()
}
