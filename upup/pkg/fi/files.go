package fi

import (
	"fmt"
	"github.com/golang/glog"
	"io"
	"os"
	"path"
	"strconv"
)

func WriteFile(destPath string, contents Resource, fileMode os.FileMode, dirMode os.FileMode) error {
	err := os.MkdirAll(path.Dir(destPath), dirMode)
	if err != nil {
		return fmt.Errorf("error creating directories for destination file %q: %v", destPath, err)
	}

	err = writeFileContents(destPath, contents, fileMode)
	if err != nil {
		return err
	}

	_, err = EnsureFileMode(destPath, fileMode)
	if err != nil {
		return err
	}

	return nil
}

func writeFileContents(destPath string, src Resource, fileMode os.FileMode) error {
	glog.Infof("Writing file %q", destPath)

	out, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("error opening destination file %q: %v", destPath, err)
	}
	defer out.Close()

	in, err := src.Open()
	if err != nil {
		return fmt.Errorf("error opening source resource for file %q: %v", destPath, err)
	}
	defer SafeClose(in)

	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("error writing file %q: %v", destPath, err)
	}
	return nil
}

func EnsureFileMode(destPath string, fileMode os.FileMode) (bool, error) {
	changed := false
	stat, err := os.Stat(destPath)
	if err != nil {
		return changed, fmt.Errorf("error getting file mode for %q: %v", destPath, err)
	}
	if stat.Mode() == fileMode {
		return changed, nil
	}
	glog.Infof("Changing file mode for %q to %s", destPath, fileMode)

	err = os.Chmod(destPath, fileMode)
	if err != nil {
		return changed, fmt.Errorf("error setting file mode for %q: %v", destPath, err)
	}
	changed = true
	return changed, nil
}

func fileHasHash(f string, expected string) (bool, error) {
	hashAlgorithm, err := determineHashAlgorithm(expected)
	if err != nil {
		return false, nil
	}

	actual, err := HashFile(f, hashAlgorithm)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if actual == expected {
		glog.V(2).Infof("Hash matched for %q: %v", f, expected)
		return true, nil
	} else {
		glog.V(2).Infof("Hash did not match for %q: actual=%v vs expected=%v", f, actual, expected)
		return false, nil
	}
}

func HashFile(f string, hashAlgorithm HashAlgorithm) (string, error) {
	glog.V(2).Infof("hashing file %q", f)

	fileAsset := NewFileResource(f)
	hash, err := HashForResource(fileAsset, hashAlgorithm)
	if err != nil {
		return "", err
	}

	return hash, nil
}

func ParseFileMode(s string, defaultMode os.FileMode) (os.FileMode, error) {
	fileMode := defaultMode
	if s != "" {
		v, err := strconv.ParseUint(s, 8, 32)
		if err != nil {
			return fileMode, fmt.Errorf("cannot parse file mode %q", s)
		}
		fileMode = os.FileMode(v)
	}
	return fileMode, nil
}

func FileModeToString(mode os.FileMode) string {
	return "0" + strconv.FormatUint(uint64(mode), 8)
}

func SafeClose(r io.Reader) {
	if r == nil {
		return
	}
	closer, ok := r.(io.Closer)
	if !ok {
		return
	}
	err := closer.Close()
	if err != nil {
		glog.Warningf("unexpected error closing stream: %v", err)
	}
}
