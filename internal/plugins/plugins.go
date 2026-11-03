// Package plugins links every built-in extension into the executable.
package plugins

import (
	_ "github.com/chilogen/jit-rules/internal/backend/jev"
	_ "github.com/chilogen/jit-rules/internal/backend/openai"
	_ "github.com/chilogen/jit-rules/internal/installer/dshpatch"
	_ "github.com/chilogen/jit-rules/internal/installer/hooksjson"
	_ "github.com/chilogen/jit-rules/internal/launcher/claudesettings"
	_ "github.com/chilogen/jit-rules/internal/launcher/codexconfig"
	_ "github.com/chilogen/jit-rules/internal/launcher/dshpatch"
	_ "github.com/chilogen/jit-rules/internal/launcher/manual"
)
