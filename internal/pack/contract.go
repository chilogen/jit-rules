// Package pack manages installable collections of judgment units.
package pack

import "github.com/chilogen/jit-rules/internal/unit"

// Manifest is a pack manifest.
type Manifest struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

// Fetched identifies fetched pack contents.
type Fetched struct {
	Dir    string
	Commit string
}

// Fetch obtains a local or git pack.
func Fetch(source, ref string) (Fetched, error) { return fetch(source, ref) }

// Validate rejects files outside the pack data format.
func Validate(dir string) error { return validate(dir) }

// ReadManifest reads a pack manifest.
func ReadManifest(dir string) (*Manifest, error) { return readManifest(dir, false) }

// Units reads and sorts all units in a pack.
func Units(dir, name string) ([]*unit.Unit, error) { return readUnits(dir, name) }

// Hash computes the content hash of a pack.
func Hash(dir string) (string, error) { return hash(dir) }

// Builtin reads the embedded core pack.
func Builtin() (*Manifest, []*unit.Unit, error) { return builtin() }

// GitDir returns the data directory for a fetched git commit.
func GitDir(name, commit string) string { return gitDir(name, commit) }
