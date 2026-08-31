package turbo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"isthmus/internal/logger"
)

const (
	DefaultChunkSize   = 2 * 1024 * 1024 // 2 MB per slice
	DefaultConcurrency = 6               // 6 parallel streams
)

var (
	ErrChecksumMismatch = errors.New("turbo chunk SHA-256 verification failed")
	ErrInvalidChunkSize = errors.New("invalid chunk size: must be > 0")
)

type ChunkManifest struct {
	Index    int    `json:"index"`
	Offset   int64  `json:"offset"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type TurboTransferManifest struct {
	Filename    string          `json:"filename"`
	TotalSize   int64           `json:"total_size"`
	TotalChunks int             `json:"total_chunks"`
	ChunkSize   int64           `json:"chunk_size"`
	Chunks      []ChunkManifest `json:"chunks"`
	FileHash    string          `json:"file_hash"`
}

type TransferProgress struct {
	TransferredBytes int64   `json:"transferred_bytes"`
	TotalBytes       int64   `json:"total_bytes"`
	CompletedChunks  int     `json:"completed_chunks"`
	TotalChunks      int     `json:"total_chunks"`
	SpeedMBps        float64 `json:"speed_mbps"`
	Percent          float64 `json:"percent"`
}

type Engine struct {
	chunkSize   int64
	concurrency int
	log         *logger.Logger
}

func NewEngine(chunkSize int64, concurrency int) *Engine {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	return &Engine{
		chunkSize:   chunkSize,
		concurrency: concurrency,
		log:         logger.WithPrefix("TurboEngine"),
	}
}

// GenerateManifest analyzes a file and splits it into a cryptographically verified chunk manifest
func (e *Engine) GenerateManifest(filePath string) (*TurboTransferManifest, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	totalSize := fi.Size()
	numChunks := int((totalSize + e.chunkSize - 1) / e.chunkSize)
	if numChunks == 0 {
		numChunks = 1
	}

	manifest := &TurboTransferManifest{
		Filename:    fi.Name(),
		TotalSize:   totalSize,
		TotalChunks: numChunks,
		ChunkSize:   e.chunkSize,
		Chunks:      make([]ChunkManifest, 0, numChunks),
	}

	fullHasher := sha256.New()
	buf := make([]byte, e.chunkSize)
	var offset int64 = 0

	for i := 0; i < numChunks; i++ {
		n, rErr := io.ReadFull(file, buf)
		if n > 0 {
			chunkData := buf[:n]
			fullHasher.Write(chunkData)

			cHash := sha256.Sum256(chunkData)
			manifest.Chunks = append(manifest.Chunks, ChunkManifest{
				Index:    i,
				Offset:   offset,
				Size:     int64(n),
				Checksum: hex.EncodeToString(cHash[:]),
			})
			offset += int64(n)
		}
		if rErr == io.EOF || rErr == io.ErrUnexpectedEOF {
			break
		}
		if rErr != nil {
			return nil, fmt.Errorf("read chunk %d: %w", i, rErr)
		}
	}

	manifest.FileHash = hex.EncodeToString(fullHasher.Sum(nil))
	e.log.Info("Prepared Turbo manifest for '%s' (%d bytes, %d chunks)", fi.Name(), totalSize, len(manifest.Chunks))
	return manifest, nil
}

// TurboCopyParallel simulates parallel multi-stream transfer pipeline locally/in-memory
func (e *Engine) TurboCopyParallel(srcPath, dstPath string, onProgress func(TransferProgress)) (*TransferProgress, error) {
	manifest, err := e.GenerateManifest(srcPath)
	if err != nil {
		return nil, err
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return nil, err
	}
	defer dstFile.Close()

	// Pre-allocate destination file size
	if err := dstFile.Truncate(manifest.TotalSize); err != nil {
		return nil, err
	}

	startTime := time.Now()
	var transferredBytes int64 = 0
	var completedChunks int = 0
	var mu sync.Mutex

	// Bounded worker pool channel
	jobs := make(chan ChunkManifest, len(manifest.Chunks))
	for _, c := range manifest.Chunks {
		jobs <- c
	}
	close(jobs)

	var wg sync.WaitGroup
	var workerErr error
	var errOnce sync.Once

	for w := 0; w < e.concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, e.chunkSize)

			for chunk := range jobs {
				// Read chunk at offset
				_, rErr := srcFile.ReadAt(buf[:chunk.Size], chunk.Offset)
				if rErr != nil && rErr != io.EOF {
					errOnce.Do(func() { workerErr = fmt.Errorf("read chunk %d: %w", chunk.Index, rErr) })
					return
				}

				// Verify SHA-256 before write
				h := sha256.Sum256(buf[:chunk.Size])
				if hex.EncodeToString(h[:]) != chunk.Checksum {
					errOnce.Do(func() { workerErr = ErrChecksumMismatch })
					return
				}

				// Write chunk at exact offset
				_, wErr := dstFile.WriteAt(buf[:chunk.Size], chunk.Offset)
				if wErr != nil {
					errOnce.Do(func() { workerErr = fmt.Errorf("write chunk %d: %w", chunk.Index, wErr) })
					return
				}

				mu.Lock()
				transferredBytes += chunk.Size
				completedChunks++
				dur := time.Since(startTime).Seconds()
				var speed float64 = 0
				if dur > 0 {
					speed = (float64(transferredBytes) / (1024 * 1024)) / dur
				}
				prog := TransferProgress{
					TransferredBytes: transferredBytes,
					TotalBytes:       manifest.TotalSize,
					CompletedChunks:  completedChunks,
					TotalChunks:      manifest.TotalChunks,
					SpeedMBps:        speed,
					Percent:          (float64(transferredBytes) / float64(manifest.TotalSize)) * 100.0,
				}
				mu.Unlock()

				if onProgress != nil {
					onProgress(prog)
				}
			}
		}()
	}

	wg.Wait()
	if workerErr != nil {
		return nil, workerErr
	}

	elapsed := time.Since(startTime).Seconds()
	finalSpeed := float64(0)
	if elapsed > 0 {
		finalSpeed = (float64(manifest.TotalSize) / (1024 * 1024)) / elapsed
	}

	finalProg := &TransferProgress{
		TransferredBytes: manifest.TotalSize,
		TotalBytes:       manifest.TotalSize,
		CompletedChunks:  manifest.TotalChunks,
		TotalChunks:      manifest.TotalChunks,
		SpeedMBps:        finalSpeed,
		Percent:          100.0,
	}

	e.log.Info("Turbo transfer of '%s' completed in %.3fs (%.2f MB/s)", manifest.Filename, elapsed, finalSpeed)
	return finalProg, nil
}
