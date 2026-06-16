package mapping

import (
	"fmt"
	"strconv"
	"strings"
)

type segment struct {
	key     string
	index   int
	isIndex bool
}

func Get(root any, path string) (any, bool, error) {
	segments, err := parsePath(path)
	if err != nil {
		return nil, false, err
	}

	current := root
	for _, segment := range segments {
		if segment.isIndex {
			items, ok := current.([]any)
			if !ok || segment.index < 0 || segment.index >= len(items) {
				return nil, false, nil
			}
			current = items[segment.index]
			continue
		}

		object, ok := current.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		value, ok := object[segment.key]
		if !ok {
			return nil, false, nil
		}
		current = value
	}

	return current, true, nil
}

func Set(root any, path string, value any) (any, error) {
	segments, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return value, nil
	}

	container := root
	if container == nil {
		if segments[0].isIndex {
			container = []any{}
		} else {
			container = map[string]any{}
		}
	}

	updated, err := setAt(container, segments, value)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func setAt(current any, segments []segment, value any) (any, error) {
	segment := segments[0]
	last := len(segments) == 1

	if segment.isIndex {
		items, ok := current.([]any)
		if !ok {
			items = []any{}
		}
		for len(items) <= segment.index {
			items = append(items, nil)
		}
		if last {
			items[segment.index] = value
			return items, nil
		}
		next := items[segment.index]
		if next == nil {
			next = newContainer(segments[1])
		}
		updated, err := setAt(next, segments[1:], value)
		if err != nil {
			return nil, err
		}
		items[segment.index] = updated
		return items, nil
	}

	object, ok := current.(map[string]any)
	if !ok {
		object = map[string]any{}
	}
	if last {
		object[segment.key] = value
		return object, nil
	}
	next := object[segment.key]
	if next == nil {
		next = newContainer(segments[1])
	}
	updated, err := setAt(next, segments[1:], value)
	if err != nil {
		return nil, err
	}
	object[segment.key] = updated
	return object, nil
}

func newContainer(next segment) any {
	if next.isIndex {
		return []any{}
	}
	return map[string]any{}
}

func parsePath(path string) ([]segment, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("json path is required")
	}
	if path == "$" {
		return nil, nil
	}
	if strings.HasPrefix(path, "$.") {
		path = strings.TrimPrefix(path, "$.")
	} else if strings.HasPrefix(path, ".") {
		path = strings.TrimPrefix(path, ".")
	} else if strings.HasPrefix(path, "$[") {
		path = strings.TrimPrefix(path, "$")
	} else if strings.HasPrefix(path, "[") {
		// Allow array root paths such as [0].name.
	} else {
		return nil, fmt.Errorf("json path must start with $ or .")
	}

	var segments []segment
	var token strings.Builder
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			if token.Len() > 0 {
				segments = append(segments, segment{key: token.String()})
				token.Reset()
			}
		case '[':
			if token.Len() > 0 {
				segments = append(segments, segment{key: token.String()})
				token.Reset()
			}
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("json path has unclosed array index")
			}
			indexText := path[i+1 : i+end]
			index, err := strconv.Atoi(indexText)
			if err != nil || index < 0 {
				return nil, fmt.Errorf("json path array index must be a non-negative integer")
			}
			segments = append(segments, segment{index: index, isIndex: true})
			i += end
		default:
			token.WriteByte(path[i])
		}
	}
	if token.Len() > 0 {
		segments = append(segments, segment{key: token.String()})
	}

	for _, segment := range segments {
		if !segment.isIndex && segment.key == "" {
			return nil, fmt.Errorf("json path contains an empty field")
		}
	}

	return segments, nil
}
