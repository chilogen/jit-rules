package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/evaluate"
	"github.com/chilogen/jit-rules/internal/pack"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var checkPack = func(
	units []*unit.Unit,
	active []*unit.Unit,
	settings profile.Settings,
) (errs []error, warns []string) {
	for _, parsed := range units {
		cases := evaluate.Test(parsed, settings, false)
		_, failed, skipped := testCounts(cases)
		if failed {
			errs = append(errs, fmt.Errorf("单元 %s 的测试未通过", parsed.ID))
		}
		if skipped {
			warns = append(warns, fmt.Sprintf("单元 %s 的模型测试已跳过", parsed.ID))
		}
	}
	history := evaluate.LoadCoHits()
	for index, parsed := range units {
		others := append(append([]*unit.Unit(nil), units[index+1:]...), active...)
		conflicts := evaluate.Conflicts(parsed, others, settings, history)
		for _, conflict := range conflicts {
			report := evaluate.FormatConflict(conflict, parsed.ID)
			if conflict.Confirmed {
				errs = append(errs, errors.New(report))
			} else {
				warns = append(warns, report)
			}
		}
	}
	return errs, warns
}

type packResult struct {
	Name   string              `json:"name"`
	Source string              `json:"source"`
	Ref    string              `json:"ref"`
	Commit string              `json:"commit"`
	Hash   string              `json:"hash"`
	Units  []map[string]string `json:"units"`
}

type packAddOptions struct {
	ref      string
	target   string
	includes []string
	excludes []string
	force    bool
}

func init() {
	command := &cobra.Command{Use: "pack", Short: "管理规则包"}
	command.AddCommand(
		newPackAddCommand(),
		newPackRemoveCommand(),
		newPackUpdateCommand(),
		newPackListCommand(),
	)
	Register(command)
}

func newPackAddCommand() *cobra.Command {
	var (
		ref      string
		target   string
		includes []string
		excludes []string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "添加规则包",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPackAdd(cmd, args[0], packAddOptions{
				ref: ref, target: target, includes: includes, excludes: excludes, force: force,
			})
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "git ref")
	cmd.Flags().StringVar(&target, "profile", "", "目标 profile")
	cmd.Flags().StringSliceVar(&includes, "include", nil, "包含的短 id glob")
	cmd.Flags().StringSliceVar(&excludes, "exclude", nil, "排除的短 id glob")
	cmd.Flags().BoolVar(&force, "force", false, "忽略测试或冲突失败")
	return cmd
}

func runPackAdd(cmd *cobra.Command, sourceArg string, options packAddOptions) error {
	defer resetCommandFlags(cmd)
	selected, err := selectProfile(options.target)
	if err != nil {
		return err
	}
	fetched, err := pack.Fetch(sourceArg, options.ref)
	if err != nil {
		return fmt.Errorf("获取包: %w", err)
	}
	prepared, warnings, err := preparePack(fetched.Dir, selected, options.force)
	if err != nil {
		return err
	}
	if prepared.manifest.Name == "core" {
		return errors.New("包名 core 保留给内置包")
	}
	for _, existing := range selected.config.Packs {
		if existing.Name == prepared.manifest.Name {
			return fmt.Errorf("包 %s 已在 profile 中", prepared.manifest.Name)
		}
	}
	source := sourceArg
	if fetched.Commit == "" {
		source = fetched.Dir
	}
	reference := profile.PackRef{
		Name: prepared.manifest.Name, Source: source, Ref: options.ref,
		Include: options.includes, Exclude: options.excludes,
	}
	if err := profile.AddPack(selected.config.Dir, reference); err != nil {
		return fmt.Errorf("更新 profile.yaml: %w", err)
	}
	selected.lock.Packs[reference.Name] = profile.LockPack{
		Source: source, Ref: options.ref, Commit: fetched.Commit,
		Hash: prepared.hash, AddedAt: time.Now().UTC(),
	}
	if err := profile.SaveLock(selected.config.Dir, selected.lock); err != nil {
		return fmt.Errorf("保存 lock: %w", err)
	}
	warnings = append(warnings,
		commitWarning(selected.config.Dir, "jitr: 添加包 "+reference.Name)...)
	result := makePackResult(reference, selected.lock.Packs[reference.Name], prepared.units)
	Output(cmd, true, result, describeUnits(prepared.units), nil, warnings)
	return nil
}

func newPackRemoveCommand() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "移除规则包",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetCommandFlags(cmd)
			selected, err := selectProfile(target)
			if err != nil {
				return err
			}
			if err := profile.RemovePack(selected.config.Dir, args[0]); err != nil {
				return fmt.Errorf("移除包: %w", err)
			}
			delete(selected.lock.Packs, args[0])
			if err := profile.SaveLock(selected.config.Dir, selected.lock); err != nil {
				return fmt.Errorf("保存 lock: %w", err)
			}
			warnings := commitWarning(selected.config.Dir, "jitr: 移除包 "+args[0])
			Output(cmd, true, map[string]string{"name": args[0]},
				fmt.Sprintf("已移除包 %s", args[0]), nil, warnings)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "profile", "", "目标 profile")
	return cmd
}

type preparedPack struct {
	manifest *pack.Manifest
	units    []*unit.Unit
	hash     string
}

func preparePack(
	dir string,
	selected *selectedProfile,
	force bool,
) (*preparedPack, []string, error) {
	if err := pack.Validate(dir); err != nil {
		return nil, nil, fmt.Errorf("校验包: %w", err)
	}
	manifest, err := pack.ReadManifest(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("读取包清单: %w", err)
	}
	units, err := pack.Units(dir, manifest.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("读取包单元: %w", err)
	}
	var warnings []string
	for _, parsed := range units {
		errs, warns := unit.Validate(parsed)
		if len(errs) > 0 {
			return nil, nil, fmt.Errorf("单元 %s 无效：%v", parsed.ID, errs)
		}
		warnings = append(warnings, warns...)
	}
	resolved, err := profile.Resolve(selected.config.Dir)
	if err != nil {
		return nil, nil, fmt.Errorf("解析 profile: %w", err)
	}
	checkErrors, checkWarnings := checkPack(
		units, activeOutsidePack(resolved.Units, manifest.Name), resolved.Settings,
	)
	warnings = append(warnings, checkWarnings...)
	if len(checkErrors) > 0 {
		if !force {
			return nil, nil, errors.Join(checkErrors...)
		}
		for _, checkErr := range checkErrors {
			warnings = append(warnings, checkErr.Error())
		}
	}
	hash, err := pack.Hash(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("计算包 hash: %w", err)
	}
	return &preparedPack{manifest: manifest, units: units, hash: hash}, warnings, nil
}

// activeOutsidePack leaves out the pack's own units so that pack update does
// not report the new version as conflicting with the one it replaces.
func activeOutsidePack(active []*unit.Unit, name string) []*unit.Unit {
	others := make([]*unit.Unit, 0, len(active))
	for _, parsed := range active {
		if !strings.HasPrefix(parsed.ID, name+"/") {
			others = append(others, parsed)
		}
	}
	return others
}

func describeUnits(units []*unit.Unit) string {
	descriptions := make([]string, 0, len(units))
	settings := profile.Defaults()
	for _, parsed := range units {
		descriptions = append(descriptions,
			strings.TrimSuffix(unit.Describe(parsed, unitThreshold(parsed, settings)), "\n"))
	}
	return strings.Join(descriptions, "\n\n")
}

func makePackResult(
	reference profile.PackRef,
	locked profile.LockPack,
	units []*unit.Unit,
) packResult {
	items := make([]map[string]string, 0, len(units))
	settings := profile.Defaults()
	for _, parsed := range units {
		items = append(items, map[string]string{
			"id":          parsed.ID,
			"description": unit.Describe(parsed, unitThreshold(parsed, settings)),
		})
	}
	return packResult{
		Name: reference.Name, Source: reference.Source, Ref: reference.Ref,
		Commit: locked.Commit, Hash: locked.Hash, Units: items,
	}
}

func localPackDir(reference profile.PackRef, selected *selectedProfile) string {
	if filepath.IsAbs(reference.Source) {
		return reference.Source
	}
	return filepath.Join(selected.config.Dir, reference.Source)
}

func resetCommandFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if values, ok := flag.Value.(pflag.SliceValue); ok && flag.DefValue == "[]" {
			_ = values.Replace(nil)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	})
}
