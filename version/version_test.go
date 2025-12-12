package version

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.etcd.io/bbolt"
)

func Test_VersionStore(t *testing.T) {
	// db
	boltdb, err := bbolt.Open(filepath.Join(t.TempDir(), "swaps"), 0700, nil)
	if err != nil {
		t.Fatal(err)
	}

	versionStore, err := NewVersionStore(boltdb)
	if err != nil {
		t.Fatal(err)
	}

	newVersion := "v0.2.0-beta"

	oldVersion, err := versionStore.GetVersion()
	if err != ErrDoesNotExist && err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", oldVersion)

	err = versionStore.SetVersion(newVersion)
	if err != nil {
		t.Fatal(err)
	}

	setVersion, err := versionStore.GetVersion()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, newVersion, setVersion)

	boltdb.Close()
}

func Test_VersionService(t *testing.T) {
	boltdb, err := bbolt.Open(filepath.Join(t.TempDir(), "swaps"), 0700, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer boltdb.Close()

	versionService, err := NewVersionService(boltdb)
	if err != nil {
		t.Fatal(err)
	}

	activeSwaps := &MockActiveSwaps{true}
	err = versionService.SafeUpgrade(activeSwaps)
	assert.Error(t, err)
	if _, ok := err.(ActiveSwapsError); !ok {
		t.Fatalf("Error not of type ActiveSwapsError")
	}

	activeSwaps = &MockActiveSwaps{false}
	err = versionService.SafeUpgrade(activeSwaps)
	assert.NoError(t, err)

}

type MockActiveSwaps struct {
	hasActiveSwaps bool
}

func (m *MockActiveSwaps) HasActiveSwaps() (bool, error) {
	return m.hasActiveSwaps, nil
}
