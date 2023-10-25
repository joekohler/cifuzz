package cmdutils

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// ValidateCorpusDirs checks if the provided corpora exist and can be
// accessed. It ensures that the paths are absolute.
func ValidateCorpusDirs(dirs []string) ([]string, error) {
	for i, d := range dirs {
		_, err := os.Stat(d)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		dirs[i], err = filepath.Abs(d)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return dirs, nil
}
