package hooksjson

func removeHandlers(root *object) {
	value, ok := root.get("hooks")
	hooks, okObject := value.(*object)
	if !ok || !okObject {
		return
	}
	keptEvents := hooks.members[:0]
	for _, event := range hooks.members {
		groups, ok := event.value.([]any)
		if !ok {
			keptEvents = append(keptEvents, event)
			continue
		}
		groups = removeFromGroups(groups)
		if len(groups) > 0 {
			event.value = groups
			keptEvents = append(keptEvents, event)
		}
	}
	hooks.members = keptEvents
	if len(hooks.members) == 0 {
		root.remove("hooks")
	}
}

func removeFromGroups(groups []any) []any {
	result := groups[:0]
	for _, value := range groups {
		group, ok := value.(*object)
		if !ok {
			result = append(result, value)
			continue
		}
		rawHandlers, exists := group.get("hooks")
		handlers, ok := rawHandlers.([]any)
		if !exists || !ok {
			result = append(result, value)
			continue
		}
		kept := handlers[:0]
		for _, handler := range handlers {
			if !isJITRHandler(handler) {
				kept = append(kept, handler)
			}
		}
		if len(kept) > 0 {
			group.set("hooks", kept)
			result = append(result, group)
		}
	}
	return result
}

func hasInstalledHandler(root *object) bool {
	value, ok := root.get("hooks")
	hooks, okObject := value.(*object)
	if !ok || !okObject {
		return false
	}
	for _, event := range hooks.members {
		groups, ok := event.value.([]any)
		if !ok {
			continue
		}
		for _, value := range groups {
			group, ok := value.(*object)
			if !ok {
				continue
			}
			rawHandlers, _ := group.get("hooks")
			handlers, _ := rawHandlers.([]any)
			for _, handler := range handlers {
				if isJITRHandler(handler) {
					return true
				}
			}
		}
	}
	return false
}
