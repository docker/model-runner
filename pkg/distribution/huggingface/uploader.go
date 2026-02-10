package huggingface

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-runner/pkg/distribution/internal/progress"
	"github.com/docker/model-runner/pkg/distribution/oci"
)

type UploadFile struct {
	LocalPath string
	RepoPath  string
	Size      int64
	ID        string
}

type uploadProgressReader struct {
	reader         io.Reader
	progressWriter io.Writer
	totalImageSize uint64
	fileSize       uint64
	fileID         string
	bytesRead      uint64
	lastReported   uint64
}

func (pr *uploadProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		pr.bytesRead += uint64(n)
		if pr.progressWriter != nil && (pr.bytesRead-pr.lastReported >= progress.MinBytesForUpdate || pr.bytesRead == pr.fileSize) {
			_ = progress.WriteProgress(pr.progressWriter, "", pr.totalImageSize, pr.fileSize, pr.bytesRead, pr.fileID, oci.ModePush)
			pr.lastReported = pr.bytesRead
		}
	}
	return n, err
}

func CollectUploadFiles(rootDir string) ([]UploadFile, int64, error) {
	root := filepath.Clean(rootDir)
	var files []UploadFile
	var totalSize int64

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve relative path: %w", err)
		}
		rel = filepath.Clean(rel)
		if rel == "." || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("invalid relative path: %s", rel)
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat file: %w", err)
		}

		repoPath := filepath.ToSlash(rel)
		files = append(files, UploadFile{
			LocalPath: path,
			RepoPath:  repoPath,
			Size:      info.Size(),
			ID:        fileIDFromPath(repoPath),
		})
		totalSize += info.Size()
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return files, totalSize, nil
}

func UploadFiles(ctx context.Context, client *Client, repo string, files []UploadFile, totalSize int64, progressWriter io.Writer) error {
	if client == nil {
		return fmt.Errorf("huggingface client is nil")
	}
	if repo == "" {
		return fmt.Errorf("repository is required")
	}

	var safeWriter io.Writer
	if progressWriter != nil {
		safeWriter = &syncWriter{w: progressWriter}
	}

	for _, file := range files {
		f, err := os.Open(file.LocalPath)
		if err != nil {
			return fmt.Errorf("open file %s: %w", file.LocalPath, err)
		}

		pr := &uploadProgressReader{
			reader:         f,
			progressWriter: safeWriter,
			totalImageSize: safeUint64(totalSize),
			fileSize:       safeUint64(file.Size),
			fileID:         file.ID,
		}

		err = client.UploadFile(ctx, repo, file.RepoPath, pr, file.Size)
		f.Close()
		if err != nil {
			return fmt.Errorf("upload %s: %w", file.RepoPath, err)
		}

		if safeWriter != nil {
			_ = progress.WriteProgress(safeWriter, "", safeUint64(totalSize), safeUint64(file.Size), safeUint64(file.Size), file.ID, oci.ModePush)
		}
	}

	return nil
}

func safeUint64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
