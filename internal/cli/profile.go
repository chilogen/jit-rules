package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chilogen/jit-rules/internal/pack"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/registry"
	"github.com/chilogen/jit-rules/internal/unit"
	"github.com/spf13/cobra"
)

type profileListItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Scope   string `json:"scope"`
	Units   int    `json:"units"`
	Applies bool   `json:"applies"`
}

type selectedProfile struct {
	config *profile.Profile
	entry  registry.Entry
	lock   *profile.Lock
}

func init() {
	command := &cobra.Command{Use: "profile", Short: "管理 profile"}
	command.AddCommand(
		newProfileInitCommand(),
		newProfileRegisterCommand(),
		newProfileUnregisterCommand(),
		newProfileListCommand(),
		newProfileShowCommand(),
	)
	Register(command)
}

func newProfileInitCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init <dir>",
		Short: "创建并注册 profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetCommandFlags(cmd)
			if name == "" {
				return errors.New("--name 是必填参数")
			}
			dir, err := absolutePath(args[0])
			if err != nil {
				return err
			}
			entry, err := profileRegistration(dir)
			if err != nil {
				return err
			}
			entries, err := registry.Load()
			if err != nil {
				return fmt.Errorf("读取注册表: %w", err)
			}
			if registeredPath(entries, dir) {
				return fmt.Errorf("profile 已注册：%s", dir)
			}
			if err := profile.Init(dir, name); err != nil {
				return fmt.Errorf("创建 profile: %w", err)
			}
			entries = append(entries, entry)
			if err := registry.Save(entries); err != nil {
				return fmt.Errorf("保存注册表: %w", err)
			}
			warnings := commitWarning(dir, "jitr: 初始化 profile")
			Output(cmd, true, map[string]any{
				"name": name, "path": dir,
				"scope": profileScope(entry),
			},
				fmt.Sprintf("已创建并注册 profile %s：%s", name, dir), nil, warnings)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "profile 名称")
	return cmd
}

func newProfileRegisterCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register <dir>",
		Short: "注册 profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetCommandFlags(cmd)
			dir, err := absolutePath(args[0])
			if err != nil {
				return err
			}
			entry, err := profileRegistration(dir)
			if err != nil {
				return err
			}
			entries, err := registry.Load()
			if err != nil {
				return fmt.Errorf("读取注册表: %w", err)
			}
			if registeredPath(entries, dir) {
				return fmt.Errorf("profile 已注册：%s", dir)
			}
			loaded, err := profile.Load(dir)
			if err != nil {
				return fmt.Errorf("加载 profile: %w", err)
			}
			lock, err := profile.LoadLock(dir)
			if err != nil {
				return fmt.Errorf("加载 lock: %w", err)
			}
			if err := validateRegisteredProfile(loaded, lock); err != nil {
				return err
			}
			entries = append(entries, entry)
			if err := registry.Save(entries); err != nil {
				return fmt.Errorf("保存注册表: %w", err)
			}
			Output(cmd, true, map[string]any{
				"name": loaded.Name, "path": dir,
				"scope": profileScope(entry),
			}, fmt.Sprintf("已注册 profile %s", loaded.Name), nil, nil)
			return nil
		},
	}
	return cmd
}

func newProfileUnregisterCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unregister <name|dir>",
		Short: "取消注册 profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := registry.Load()
			if err != nil {
				return fmt.Errorf("读取注册表: %w", err)
			}
			index, loaded, err := findRegisteredProfile(entries, args[0])
			if err != nil {
				return err
			}
			entries = append(entries[:index], entries[index+1:]...)
			if err := registry.Save(entries); err != nil {
				return fmt.Errorf("保存注册表: %w", err)
			}
			Output(cmd, true, map[string]string{"name": loaded.Name, "path": loaded.Dir},
				fmt.Sprintf("已取消注册 profile %s", loaded.Name), nil, nil)
			return nil
		},
	}
}

func newProfileListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出 profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("读取当前目录: %w", err)
			}
			entries, err := registry.Load()
			if err != nil {
				return fmt.Errorf("读取注册表: %w", err)
			}
			applicable := registry.Applicable(entries, cwd)
			resolved, err := profile.Resolve(cwd)
			if err != nil {
				return fmt.Errorf("解析 profile: %w", err)
			}
			counts := map[string]int{}
			for _, state := range resolved.States {
				if state.Status == "active" {
					counts[state.Profile]++
				}
			}
			items := make([]profileListItem, 0, len(entries))
			var lines []string
			for _, entry := range entries {
				loaded, err := profile.Load(entry.Path)
				if err != nil {
					return fmt.Errorf("加载 profile %s: %w", entry.Path, err)
				}
				item := profileListItem{
					Name:  loaded.Name,
					Path:  entry.Path,
					Scope: profileScope(entry),
					Units: counts[loaded.Name], Applies: containsEntry(applicable, entry.Path),
				}
				items = append(items, item)
				lines = append(lines, fmt.Sprintf(
					"%s\t%s\tscope=%s\tunits=%d\tapplies=%t",
					item.Name, item.Path, item.Scope,
					item.Units, item.Applies,
				))
			}
			Output(cmd, true, items, strings.Join(lines, "\n"), resolved.Errors, nil)
			if len(resolved.Errors) > 0 {
				return Silent(1)
			}
			return nil
		},
	}
}

func newProfileShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [profile]",
		Short: "显示 profile 的生效单元",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			selected, err := selectProfile(target)
			if err != nil {
				return err
			}
			resolveAt, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("读取当前目录: %w", err)
			}
			if target != "" && !registry.IsGlobal(selected.entry) {
				resolveAt = registry.Scope(selected.entry)
			}
			resolved, err := profile.Resolve(resolveAt)
			if err != nil {
				return fmt.Errorf("解析 profile: %w", err)
			}
			var descriptions []string
			var data []map[string]string
			for _, state := range resolved.States {
				if state.Status != "active" || state.Profile != selected.config.Name {
					continue
				}
				description := unit.Describe(state.Unit, unitThreshold(state.Unit, resolved.Settings))
				descriptions = append(descriptions, strings.TrimSuffix(description, "\n"))
				data = append(data, map[string]string{
					"id": state.ID, "description": description,
				})
			}
			Output(cmd, true, data, strings.Join(descriptions, "\n\n"), resolved.Errors, nil)
			if len(resolved.Errors) > 0 {
				return Silent(1)
			}
			return nil
		},
	}
}

func selectProfile(value string) (*selectedProfile, error) {
	entries, err := registry.Load()
	if err != nil {
		return nil, fmt.Errorf("读取注册表: %w", err)
	}
	var entry registry.Entry
	if value == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("读取当前目录: %w", err)
		}
		applicable := registry.Applicable(entries, cwd)
		if len(applicable) == 0 {
			return nil, errors.New("当前目录没有可用的 profile，请先运行 jitr profile init")
		}
		entry = applicable[0]
	} else {
		index, _, err := findRegisteredProfile(entries, value)
		if err != nil {
			return nil, err
		}
		entry = entries[index]
	}
	loaded, err := profile.Load(entry.Path)
	if err != nil {
		return nil, fmt.Errorf("加载 profile: %w", err)
	}
	lock, err := profile.LoadLock(entry.Path)
	if err != nil {
		return nil, fmt.Errorf("加载 lock: %w", err)
	}
	return &selectedProfile{config: loaded, entry: entry, lock: lock}, nil
}

func findRegisteredProfile(entries []registry.Entry, value string) (int, *profile.Profile, error) {
	path, _ := absolutePath(value)
	for index, entry := range entries {
		loaded, err := profile.Load(entry.Path)
		if err != nil {
			continue
		}
		if loaded.Name == value || filepath.Clean(entry.Path) == path {
			return index, loaded, nil
		}
	}
	return -1, nil, fmt.Errorf("找不到已注册的 profile %s", value)
}

func validateRegisteredProfile(loaded *profile.Profile, lock *profile.Lock) error {
	for id, locked := range lock.Units {
		shortID := strings.TrimPrefix(id, loaded.Name+"/")
		parsed, err := unit.ParseFile(filepath.Join(loaded.Dir, "units", shortID+".md"), loaded.Name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("校验单元 %s: %w", id, err)
		}
		if parsed.Hash != locked.Hash {
			continue
		}
		if errs, _ := unit.Validate(parsed); len(errs) > 0 {
			return fmt.Errorf("单元 %s 无效：%v", id, errs)
		}
	}
	for _, reference := range loaded.Packs {
		var dir string
		if reference.Source == "builtin" {
			_, units, err := pack.Builtin()
			if err != nil {
				return fmt.Errorf("读取内置包: %w", err)
			}
			if err := validateUnits(units); err != nil {
				return err
			}
			continue
		}
		locked, ok := lock.Packs[reference.Name]
		if !ok {
			continue
		}
		if locked.Commit != "" {
			dir = pack.GitDir(reference.Name, locked.Commit)
		} else {
			dir = reference.Source
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(loaded.Dir, dir)
			}
		}
		hash, err := pack.Hash(dir)
		if err != nil || hash != locked.Hash {
			continue
		}
		units, err := pack.Units(dir, reference.Name)
		if err != nil {
			return fmt.Errorf("读取包 %s: %w", reference.Name, err)
		}
		if err := validateUnits(units); err != nil {
			return err
		}
	}
	return nil
}

func validateUnits(units []*unit.Unit) error {
	for _, parsed := range units {
		if errs, _ := unit.Validate(parsed); len(errs) > 0 {
			return fmt.Errorf("单元 %s 无效：%v", parsed.ID, errs)
		}
	}
	return nil
}
