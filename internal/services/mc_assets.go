package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"HRPMLW/internal/models"
)

const assetDownloadWorkers = 8

// getAsset retries temporary mirror and CDN failures without switching sources.
func (b *BMCLAPIService) getAsset(ctx context.Context, url string, read func(io.Reader) error) error {
	const attempts = 3
	var err error
	for attempt := range attempts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attemptCtx, stop := context.WithTimeout(ctx, 30*time.Second)
		var resp *http.Response
		resp, err = b.getCtx(attemptCtx, url)
		if err == nil {
			status := resp.StatusCode
			if status == http.StatusOK {
				err = read(resp.Body)
			} else {
				err = fmt.Errorf("HTTP %d", status)
			}
			resp.Body.Close()
			stop()
			if status == http.StatusOK {
				var pathErr *os.PathError
				var syntaxErr *json.SyntaxError
				var typeErr *json.UnmarshalTypeError
				if err == nil || errors.As(err, &pathErr) || errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
					return err
				}
			} else if status < 500 && status != http.StatusTooManyRequests &&
				status != http.StatusRequestTimeout && status != http.StatusForbidden {
				return err
			}
			// A fresh request to BMCLAPI can select a different CDN node
			// when the original redirect destination rejected the request.
		} else {
			stop()
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(250 * time.Millisecond << attempt)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return err
}

func assetOnDisk(path string, size int64) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == size
}

func (b *BMCLAPIService) downloadAssetObject(ctx context.Context, url, path string, size int64) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return b.getAsset(ctx, url, func(body io.Reader) error {
		file, err := os.CreateTemp(dir, ".asset-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		n, copyErr := io.Copy(file, body)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != size {
			return fmt.Errorf("expected %d bytes, received %d", size, n)
		}
		if err := os.Rename(file.Name(), path); err != nil && !assetOnDisk(path, size) {
			return err
		}
		return nil
	})
}

// DownloadAssets fetches missing content-addressed objects in parallel.
func (b *BMCLAPIService) DownloadAssets(ctx context.Context, assetIndex *BMCLAssetIndex, assetsDir string, progressCallback func(name string, downloaded, total int64)) *models.ApiResponse {
	type object struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`
	}
	var objects map[string]object
	assetIndexURL := b.rewriteGameURL(assetIndex.URL)
	if err := b.getAsset(ctx, assetIndexURL, func(body io.Reader) error {
		var index struct {
			Objects map[string]object `json:"objects"`
		}
		if err := json.NewDecoder(body).Decode(&index); err != nil {
			return err
		}
		if index.Objects == nil {
			return errors.New("missing objects in asset index")
		}
		objects = index.Objects
		return nil
	}); err != nil {
		return &models.ApiResponse{Success: false, Error: fmt.Sprintf("failed to download asset index %s: %v", assetIndex.ID, cancelled(ctx, err))}
	}

	type job struct {
		name, path, relativeURL string
		size                    int64
	}
	var pending []job
	var total, downloaded int64
	seen := make(map[string]struct{}, len(objects))
	for name, obj := range objects {
		if ctx.Err() != nil {
			return &models.ApiResponse{Success: false, Error: cancelled(ctx, ctx.Err()).Error()}
		}
		if _, err := hex.DecodeString(obj.Hash); err != nil || len(obj.Hash) < 2 || obj.Size < 0 {
			return &models.ApiResponse{Success: false, Error: fmt.Sprintf("invalid asset index entry for %s", name)}
		}
		if _, ok := seen[obj.Hash]; ok {
			continue
		}
		seen[obj.Hash] = struct{}{}
		total += obj.Size
		subDir := obj.Hash[:2]
		path := filepath.Join(assetsDir, "objects", subDir, obj.Hash)
		if assetOnDisk(path, obj.Size) {
			downloaded += obj.Size
			continue
		}
		pending = append(pending, job{name, path, "/" + subDir + "/" + obj.Hash, obj.Size})
	}

	workCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	var next atomic.Uint64
	var workers sync.WaitGroup
	var progressMu sync.Mutex
	for range min(b.assetWorkers, len(pending)) {
		workers.Go(func() {
			for workCtx.Err() == nil {
				i := next.Add(1) - 1
				if i >= uint64(len(pending)) {
					return
				}
				task := pending[i]
				if err := b.downloadAssetObject(workCtx, b.assetURL(task.relativeURL), task.path, task.size); err != nil {
					stop(fmt.Errorf("failed to download asset %s (%s): %w", task.name, task.relativeURL, err))
					return
				}
				if progressCallback != nil {
					progressMu.Lock()
					downloaded += task.size
					progressCallback(task.name, downloaded, total)
					progressMu.Unlock()
				}
			}
		})
	}
	workers.Wait()
	if err := context.Cause(workCtx); err != nil {
		return &models.ApiResponse{Success: false, Error: cancelled(ctx, err).Error()}
	}
	return &models.ApiResponse{Success: true}
}
