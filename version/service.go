package version

import (
	"fmt"

	"go.etcd.io/bbolt"
)

const (
	version = "v0.2"
)

type ActiveSwapGetter interface {
	HasActiveSwaps() (bool, error)
}

type VersionService struct {
	versionStore *versionStore
}

func NewVersionService(boltdb *bbolt.DB) (*VersionService, error) {
	versionStore, err := NewVersionStore(boltdb)
	if err != nil {
		return nil, err
	}
	return &VersionService{versionStore: versionStore}, nil
}

// SafeUpgrade upgrades the peerswap version, only if no active swaps are running
func (vs *VersionService) SafeUpgrade(swapService ActiveSwapGetter) error {
	// first check if we need to upgrade
	currentVersion, err := vs.versionStore.GetVersion()
	if err != ErrDoesNotExist && err != nil {
		return err
	}

	// we're running the same version as before and can safely continue
	if err != ErrDoesNotExist && currentVersion == version {
		return nil
	}

	// check if we have active swaps
	hasActiveSwaps, err := swapService.HasActiveSwaps()
	if err != nil {
		return err
	}

	// if we active swap, abort peerswap and notify the user to upgrade
	if hasActiveSwaps {
		return ActiveSwapsError{currentVersion}
	}

	// Now it's safe to upgrade
	err = vs.versionStore.SetVersion(version)
	if err != nil {
		return err
	}

	return nil

}

// GetCurrentVersion returns the hardcoded implementation version, sometimes
// also referred to as database version.
func GetCurrentVersion() string {
	return version
}

type ActiveSwapsError struct {
	version string
}

func (a ActiveSwapsError) Error() string {
	return fmt.Sprintf("Can't upgrade because of active swaps. Please downgrade peerswap to version %s", a.version)
}
