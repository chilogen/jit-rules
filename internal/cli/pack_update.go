package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/pack"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/spf13/cobra"
)

type packListItem struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	Units  int    `json:"units"`
	Status string `json:"status"`
}

type pendingPackUpdate struct {
	reference profile.PackRef
	locked    profile.LockPack
	prepared  *preparedPack
	warnings  []string
}

type packUpdateOptions struct {
	name      string
	ref       string
	target    string
	force     bool
	changeRef bool
}

func newPackUpdateCommand() *cobra.Command {
	var (
		ref    string
		target string
		force  bool
	)
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "更新规则包",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runPackUpdate(cmd, packUpdateOptions{
				name: name, ref: ref, target: target, force: force,
				changeRef: cmd.Flags().Changed("ref"),
			})
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "新的 git ref")
	cmd.Flags().StringVar(&target, "profile", "", "目标 profile")
	cmd.Flags().BoolVar(&force, "force", false, "忽略测试或冲突失败")
	return cmd
}

func runPackUpdate(cmd *cobra.Command, options packUpdateOptions) error {
	defer resetCommandFlags(cmd)
	selected, err := selectProfile(options.target)
	if err != nil {
		return err
	}
	references, err := selectedPackRefs(selected.config.Packs, options.name)
	if err != nil {
		return err
	}
	pending, err := preparePackUpdates(
		selected, references, options.ref, options.changeRef, options.force,
	)
	if err != nil {
		return err
	}
	var warnings, descriptions []string
	var results []packResult
	for _, update := range pending {
		if options.changeRef {
			if err := profile.SetPackRef(
				selected.config.Dir, update.reference.Name, update.reference.Ref,
			); err != nil {
				return fmt.Errorf("更新包 ref: %w", err)
			}
		}
		selected.lock.Packs[update.reference.Name] = update.locked
		warnings = append(warnings, update.warnings...)
		descriptions = append(descriptions, describeUnits(update.prepared.units))
		results = append(results,
			makePackResult(update.reference, update.locked, update.prepared.units))
	}
	if len(pending) > 0 {
		if err := profile.SaveLock(selected.config.Dir, selected.lock); err != nil {
			return fmt.Errorf("保存 lock: %w", err)
		}
		warnings = append(warnings,
			commitWarning(selected.config.Dir, "jitr: 更新规则包")...)
	}
	Output(cmd, true, results, strings.Join(descriptions, "\n\n"), nil, warnings)
	return nil
}

func selectedPackRefs(
	references []profile.PackRef,
	name string,
) ([]profile.PackRef, error) {
	if name == "" {
		result := make([]profile.PackRef, 0, len(references))
		for _, reference := range references {
			if reference.Source != "builtin" {
				result = append(result, reference)
			}
		}
		return result, nil
	}
	for _, reference := range references {
		if reference.Name == name {
			if reference.Source == "builtin" {
				return []profile.PackRef{}, nil
			}
			return []profile.PackRef{reference}, nil
		}
	}
	return nil, fmt.Errorf("包 %s 不在 profile 中", name)
}

func preparePackUpdates(
	selected *selectedProfile,
	references []profile.PackRef,
	newRef string,
	changeRef bool,
	force bool,
) ([]pendingPackUpdate, error) {
	pending := make([]pendingPackUpdate, 0, len(references))
	for _, reference := range references {
		nextRef := reference.Ref
		if changeRef {
			nextRef = newRef
		}
		source := reference.Source
		if locked, ok := selected.lock.Packs[reference.Name]; ok && locked.Commit == "" {
			source = localPackDir(reference, selected)
		}
		fetched, err := pack.Fetch(source, nextRef)
		if err != nil {
			return nil, fmt.Errorf("获取包 %s: %w", reference.Name, err)
		}
		prepared, warnings, err := preparePack(fetched.Dir, selected, force)
		if err != nil {
			return nil, fmt.Errorf("检查包 %s: %w", reference.Name, err)
		}
		if prepared.manifest.Name != reference.Name {
			return nil, fmt.Errorf(
				"包名从 %s 变成了 %s", reference.Name, prepared.manifest.Name,
			)
		}
		reference.Ref = nextRef
		lockedSource := reference.Source
		if current, ok := selected.lock.Packs[reference.Name]; ok && current.Source != "" {
			lockedSource = current.Source
		}
		pending = append(pending, pendingPackUpdate{
			reference: reference,
			locked: profile.LockPack{
				Source: lockedSource, Ref: nextRef, Commit: fetched.Commit,
				Hash: prepared.hash, AddedAt: time.Now().UTC(),
			},
			prepared: prepared,
			warnings: warnings,
		})
	}
	return pending, nil
}

func newPackListCommand() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出规则包",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			defer resetCommandFlags(cmd)
			selected, err := selectProfile(target)
			if err != nil {
				return err
			}
			items := make([]packListItem, 0, len(selected.config.Packs))
			lines := make([]string, 0, len(selected.config.Packs))
			for _, reference := range selected.config.Packs {
				item, err := inspectPack(reference, selected)
				if err != nil {
					return err
				}
				items = append(items, item)
				lines = append(lines, fmt.Sprintf(
					"%s\t%s\tref=%s\tcommit=%s\tunits=%d\t%s",
					item.Name, item.Source, displayRef(item.Ref),
					displayCommit(item.Commit), item.Units, item.Status,
				))
			}
			Output(cmd, true, items, strings.Join(lines, "\n"), nil, nil)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "profile", "", "目标 profile")
	return cmd
}

func inspectPack(
	reference profile.PackRef,
	selected *selectedProfile,
) (packListItem, error) {
	item := packListItem{
		Name: reference.Name, Source: reference.Source, Ref: reference.Ref, Status: "ok",
	}
	if reference.Source == "builtin" {
		_, units, err := pack.Builtin()
		if err != nil {
			return item, fmt.Errorf("读取内置包: %w", err)
		}
		item.Units = len(units)
		return item, nil
	}
	locked, ok := selected.lock.Packs[reference.Name]
	if !ok {
		item.Status = "missing"
		return item, nil
	}
	item.Commit = locked.Commit
	dir := localPackDir(reference, selected)
	if locked.Commit != "" {
		dir = pack.GitDir(reference.Name, locked.Commit)
	}
	hash, err := pack.Hash(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			item.Status = "missing"
			return item, nil
		}
		return item, fmt.Errorf("检查包 %s: %w", reference.Name, err)
	}
	if hash != locked.Hash {
		item.Status = "modified"
		return item, nil
	}
	units, err := pack.Units(dir, reference.Name)
	if err != nil {
		return item, fmt.Errorf("读取包 %s: %w", reference.Name, err)
	}
	item.Units = len(units)
	return item, nil
}

func displayCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return displayRef(commit)
}

func displayRef(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
