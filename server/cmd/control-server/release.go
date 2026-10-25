package main

import (
	"appcontrol/server/internal/config"
	"appcontrol/server/internal/release"
	"io"
	"os"
)

func configureRelease(settings config.Config) (*release.Document, error) {
	if settings.ReleaseManifestPath == "" && settings.ReleasePublicKeyPath == "" && settings.ReleaseOrigin == "" && settings.ReleaseCertificate == "" {
		return nil, nil
	}
	read := func(path string, maximum int64) ([]byte, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, release.ErrInvalid
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
			return nil, release.ErrInvalid
		}
		raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
		if err != nil || int64(len(raw)) > maximum {
			return nil, release.ErrInvalid
		}
		return raw, nil
	}
	raw, err := read(settings.ReleaseManifestPath, 64*1024)
	if err != nil {
		return nil, release.ErrInvalid
	}
	public, err := read(settings.ReleasePublicKeyPath, 128)
	if err != nil {
		return nil, release.ErrInvalid
	}
	return release.NewDocument(raw, public, settings.ReleaseOrigin, settings.ReleaseCertificate, settings.ReleaseMinimumSequence)
}
