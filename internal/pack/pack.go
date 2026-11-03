package pack

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/chilogen/jit-rules/internal/unit"
	"gopkg.in/yaml.v3"
)

var packNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func readManifest(dir string, allowCore bool) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "pack.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read pack manifest: %w", err)
	}
	return parseManifest(data, allowCore)
}

func parseManifest(data []byte, allowCore bool) (*Manifest, error) {
	var manifest Manifest
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse pack manifest: %w", err)
	}
	if !packNamePattern.MatchString(manifest.Name) {
		return nil, fmt.Errorf("invalid pack name %q", manifest.Name)
	}
	if manifest.Name == "core" && !allowCore {
		return nil, errors.New("pack name core is reserved")
	}
	if manifest.Version == "" || manifest.Description == "" {
		return nil, errors.New("pack version and description are required")
	}
	return &manifest, nil
}

func readUnits(dir, name string) ([]*unit.Unit, error) {
	files, err := filepath.Glob(filepath.Join(dir, "units", "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	units := make([]*unit.Unit, 0, len(files))
	for _, file := range files {
		parsed, err := unit.ParseFile(file, name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		units = append(units, parsed)
	}
	return units, nil
}

func validate(dir string) error {
	if _, err := readManifest(dir, false); err != nil {
		return err
	}
	var rejected []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		allowed := relative == "pack.yaml" || relative == "README.md" ||
			relative == "LICENSE" || relative == "units" && entry.IsDir() ||
			strings.HasPrefix(relative, "units/") && !entry.IsDir() &&
				entry.Type().IsRegular() && strings.HasSuffix(relative, ".md") &&
				!strings.Contains(strings.TrimPrefix(relative, "units/"), "/")
		if !allowed {
			rejected = append(rejected, relative)
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect pack: %w", err)
	}
	if len(rejected) > 0 {
		sort.Strings(rejected)
		return fmt.Errorf("pack contains unsupported paths: %s", strings.Join(rejected, ", "))
	}
	units, err := readUnits(dir, filepath.Base(dir))
	if err != nil {
		return err
	}
	for _, parsed := range units {
		if errs, _ := unit.Validate(parsed); len(errs) > 0 {
			return fmt.Errorf("%s: %v", parsed.File, errs)
		}
	}
	return nil
}

func hash(dir string) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "units", "*.md"))
	if err != nil {
		return "", err
	}
	files = append(files, filepath.Join(dir, "pack.yaml"))
	sort.Slice(files, func(i, j int) bool {
		left, _ := filepath.Rel(dir, files[i])
		right, _ := filepath.Rel(dir, files[j])
		return filepath.ToSlash(left) < filepath.ToSlash(right)
	})
	var content strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("hash %s: %w", file, err)
		}
		relative, err := filepath.Rel(dir, file)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&content, "%s\t%x\n", filepath.ToSlash(relative), sum)
	}
	sum := sha256.Sum256([]byte(content.String()))
	return fmt.Sprintf("sha256:%x", sum), nil
}

func builtin() (*Manifest, []*unit.Unit, error) {
	const root = "packs/core"
	data, err := assets.FS.ReadFile(root + "/pack.yaml")
	if err != nil {
		return nil, nil, err
	}
	manifest, err := parseManifest(data, true)
	if err != nil {
		return nil, nil, err
	}
	entries, err := fs.ReadDir(assets.FS, root+"/units")
	if err != nil {
		return nil, nil, err
	}
	var units []*unit.Unit
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		file := root + "/units/" + entry.Name()
		data, err := assets.FS.ReadFile(file)
		if err != nil {
			return nil, nil, err
		}
		parsed, err := unit.Parse(data, "builtin/core/units/"+entry.Name(), manifest.Name)
		if err != nil {
			return nil, nil, err
		}
		units = append(units, parsed)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })
	return manifest, units, nil
}

func gitDir(name, commit string) string {
	return filepath.Join(paths.Data(), "packs", name, commit)
}

func fetch(source, ref string) (result Fetched, resultErr error) {
	if source == "builtin" {
		return Fetched{}, errors.New("builtin pack cannot be fetched")
	}
	if info, err := os.Stat(source); err == nil {
		if !info.IsDir() {
			return Fetched{}, fmt.Errorf("找不到目录 %s", source)
		}
		absolute, err := filepath.Abs(source)
		if err != nil {
			return Fetched{}, err
		}
		return Fetched{Dir: absolute}, nil
	}
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		return Fetched{}, fmt.Errorf("找不到目录 %s", source)
	}
	return fetchGit(source, ref)
}

func fetchGit(source, ref string) (result Fetched, resultErr error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return Fetched{}, errors.New("需要安装 git 才能从 git 仓库安装包")
	}
	tempRoot := filepath.Join(paths.Data(), "packs", ".tmp")
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return Fetched{}, err
	}
	temp, err := os.MkdirTemp(tempRoot, "fetch-*")
	if err != nil {
		return Fetched{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = os.RemoveAll(temp)
		}
	}()
	if output, err := runGit(git, "", "clone", source, temp); err != nil {
		return Fetched{}, fmt.Errorf("git clone: %w: %s", err, output)
	}
	commit := ""
	if ref == "" {
		commit, err = gitOutput(git, temp, "rev-parse", "HEAD")
	} else {
		for _, candidate := range []string{"origin/" + ref + "^{commit}", ref + "^{commit}"} {
			commit, err = gitOutput(git, temp, "rev-parse", "--verify", "--quiet", candidate)
			if err == nil {
				break
			}
		}
		if err != nil {
			return Fetched{}, fmt.Errorf("找不到 ref %s", ref)
		}
		if output, checkoutErr := runGit(git, temp, "checkout", "--detach", commit); checkoutErr != nil {
			return Fetched{}, fmt.Errorf("git checkout: %w: %s", checkoutErr, output)
		}
	}
	if err := os.RemoveAll(filepath.Join(temp, ".git")); err != nil {
		return Fetched{}, err
	}
	manifest, err := readManifest(temp, false)
	if err != nil {
		return Fetched{}, err
	}
	target := gitDir(manifest.Name, commit)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Fetched{}, err
	}
	if err := os.RemoveAll(target); err != nil {
		return Fetched{}, err
	}
	if err := os.Rename(temp, target); err != nil {
		return Fetched{}, err
	}
	return Fetched{Dir: target, Commit: commit}, nil
}

func gitOutput(git, dir string, args ...string) (string, error) {
	output, err := runGit(git, dir, args...)
	return strings.TrimSpace(output), err
}

func runGit(git, dir string, args ...string) (string, error) {
	command := exec.Command(git, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
