package services

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
)

func (s *ModrinthService) Install(ctx context.Context, gameDir string, target ModrinthTarget, projectID, versionID, fingerprint string) error {
	plan, err := s.Plan(ctx, target, projectID, versionID)
	if err != nil {
		return err
	}
	return installOnlineMods(ctx, gameDir, plan, fingerprint, s.get)
}

func installOnlineMods(ctx context.Context, gameDir string, plan ModrinthPlan, fingerprint string, download func(context.Context, string) (*http.Response, error)) error {
	if fingerprint == "" || plan.Fingerprint != fingerprint {
		return fmt.Errorf("online mod files changed; review the installation again")
	}
	root, err := openMods(gameDir, true)
	if err != nil {
		return err
	}
	defer root.Close()

	staged := make(map[string]string)
	defer func() {
		for _, tmp := range staged {
			_ = root.Remove(tmp)
		}
	}()
	for _, file := range plan.Files {
		present, err := existingModrinthMod(root, file)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		tmp := ".samuel-mod-" + rand.Text() + ".part"
		if err := stageOnlineMod(ctx, root, tmp, file, download); err != nil {
			return fmt.Errorf("download %s: %w", file.Name, err)
		}
		staged[file.Name] = tmp
	}

	modsMu.Lock()
	defer modsMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	var pending []ModrinthPlanFile
	for _, file := range plan.Files {
		present, err := existingModrinthMod(root, file)
		if err != nil {
			return err
		}
		if !present {
			if staged[file.Name] == "" {
				return fmt.Errorf("%s changed during installation; try again", file.Name)
			}
			pending = append(pending, file)
		}
	}
	var added []string
	for _, file := range pending {
		if err := publishModrinthFile(root, staged[file.Name], file.Name); err != nil {
			for _, name := range added {
				err = errors.Join(err, root.Remove(name))
			}
			return err
		}
		added = append(added, file.Name)
	}
	return nil
}

func publishModrinthFile(root *os.Root, tmp, name string) error {
	// Link publishes atomically. Portable drives may not support hard links;
	// on those, copy the already verified file with an exclusive create.
	linkErr := root.Link(tmp, name)
	if linkErr == nil || errors.Is(linkErr, os.ErrExist) {
		return linkErr
	}
	input, err := root.Open(tmp)
	if err != nil {
		return errors.Join(linkErr, err)
	}
	defer input.Close()
	out, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Join(linkErr, err)
	}
	_, copyErr := io.Copy(out, input)
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return errors.Join(err, root.Remove(name))
	}
	return nil
}

func existingModrinthMod(root *os.Root, file ModrinthPlanFile) (bool, error) {
	if err := checkModDestination(root, file.Name+disabledModSuffix); err != nil {
		return false, err
	}
	info, err := root.Lstat(file.Name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode().IsRegular() && info.Size() == file.Size {
		input, err := root.Open(file.Name)
		if err != nil {
			return false, err
		}
		defer input.Close()
		h, digest := onlineModDigest(file)
		n, err := io.Copy(h, io.LimitReader(input, file.Size+1))
		if err != nil {
			return false, err
		}
		if n == file.Size && hex.EncodeToString(h.Sum(nil)) == digest {
			return true, nil
		}
	}
	return false, fmt.Errorf("a different file named %q already exists", file.Name)
}

func onlineModDigest(file ModrinthPlanFile) (hash.Hash, string) {
	if file.SHA512 != "" {
		return sha512.New(), file.SHA512
	}
	return sha1.New(), file.SHA1
}

func stageOnlineMod(ctx context.Context, root *os.Root, tmp string, file ModrinthPlanFile, download func(context.Context, string) (*http.Response, error)) error {
	resp, err := download(ctx, file.url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	h, digest := onlineModDigest(file)
	n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, file.Size+1))
	err = errors.Join(copyErr, out.Close())
	if err == nil && (n != file.Size || hex.EncodeToString(h.Sum(nil)) != digest) {
		err = fmt.Errorf("size or checksum mismatch")
	}
	if err != nil {
		return errors.Join(err, root.Remove(tmp))
	}
	return nil
}
