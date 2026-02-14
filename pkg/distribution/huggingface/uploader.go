package huggingface

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	OID       string
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

	const lfsThreshold = 10 * 1024 * 1024

	var safeWriter io.Writer
	if progressWriter != nil {
		safeWriter = &syncWriter{w: progressWriter}
	}

	var lfsFiles []UploadFile
	var directFiles []UploadFile
	for i := range files {
		if files[i].Size >= lfsThreshold {
			oid, err := computeFileOID(files[i].LocalPath)
			if err != nil {
				return fmt.Errorf("compute oid for %s: %w", files[i].RepoPath, err)
			}
			files[i].OID = oid
			lfsFiles = append(lfsFiles, files[i])
		} else {
			directFiles = append(directFiles, files[i])
		}
	}

	var lfsCommitFiles []LFSCommitFile
	if len(lfsFiles) > 0 {
		objects := make([]LFSBatchObject, 0, len(lfsFiles))
		for _, file := range lfsFiles {
			objects = append(objects, LFSBatchObject{OID: file.OID, Size: file.Size})
		}
		batchResp, err := client.LFSBatch(ctx, repo, objects)
		if err != nil {
			return fmt.Errorf("lfs batch: %w", err)
		}
		objByOID := make(map[string]LFSObject, len(batchResp.Objects))
		for _, obj := range batchResp.Objects {
			objByOID[obj.OID] = obj
		}

		for _, file := range lfsFiles {
			obj, ok := objByOID[file.OID]
			if !ok {
				return fmt.Errorf("missing lfs response for %s", file.RepoPath)
			}
			if obj.Error != nil {
				return fmt.Errorf("lfs error for %s: %s", file.RepoPath, obj.Error.Message)
			}

			uploadAction, hasUpload := obj.Actions["upload"]
			if hasUpload && uploadAction.Href != "" {
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
				err = client.UploadLFSObject(ctx, uploadAction, pr, file.Size)
				f.Close()
				if err != nil {
					return fmt.Errorf("upload lfs %s: %w", file.RepoPath, err)
				}

				if verifyAction, ok := obj.Actions["verify"]; ok {
					if err := client.VerifyLFSObject(ctx, verifyAction, file.OID, file.Size); err != nil {
						return fmt.Errorf("verify lfs %s: %w", file.RepoPath, err)
					}
				}

				if safeWriter != nil {
					_ = progress.WriteProgress(safeWriter, "", safeUint64(totalSize), safeUint64(file.Size), safeUint64(file.Size), file.ID, oci.ModePush)
				}
			}

			lfsCommitFiles = append(lfsCommitFiles, LFSCommitFile{
				Path: file.RepoPath,
				Algo: "sha256",
				OID:  file.OID,
				Size: file.Size,
			})
		}
	}

	var commitFiles []CommitFile
	var openFiles []*os.File
	defer func() {
		for _, f := range openFiles {
			f.Close()
		}
	}()

	for _, file := range directFiles {
		f, err := os.Open(file.LocalPath)
		if err != nil {
			return fmt.Errorf("open file %s: %w", file.LocalPath, err)
		}
		openFiles = append(openFiles, f)

		pr := &uploadProgressReader{
			reader:         f,
			progressWriter: safeWriter,
			totalImageSize: safeUint64(totalSize),
			fileSize:       safeUint64(file.Size),
			fileID:         file.ID,
		}

		commitFiles = append(commitFiles, CommitFile{
			RepoPath: file.RepoPath,
			Content:  pr,
		})
	}

	if err := client.CreateCommit(ctx, repo, "Upload model via docker model push", commitFiles, lfsCommitFiles); err != nil {
		return fmt.Errorf("create commit: %w", err)
	}

	for _, file := range directFiles {
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

func computeFileOID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
