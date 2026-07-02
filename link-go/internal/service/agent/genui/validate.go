package genui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// catalogSet 是 Catalog 的集合形式，供 O(1) 白名单校验。
var catalogSet = func() map[string]bool {
	s := make(map[string]bool, len(Catalog))
	for _, c := range Catalog {
		s[c] = true
	}
	return s
}()

// Validate 校验一份 UISpec 的结构完整性与绑定可解析性：
//  1. 组件类型必须在 catalog 白名单内；
//  2. 有且仅有一个 id=="root" 的节点；
//  3. 所有 children 引用的 id 必须存在；
//  4. 所有 Props 中的 {path} 绑定必须能在 DataModel 上解析（RFC6901 JSON Pointer）。
//
// 供 LLM 生成路径（Level 2）在信任前做守门；不通过则回退模板。
func Validate(spec *UISpec) error {
	if len(spec.Components) == 0 {
		return fmt.Errorf("empty components")
	}

	ids := make(map[string]bool, len(spec.Components))
	rootCount := 0
	for _, c := range spec.Components {
		if c.ID == "" {
			return fmt.Errorf("component missing id")
		}
		if ids[c.ID] {
			return fmt.Errorf("duplicate component id: %s", c.ID)
		}
		ids[c.ID] = true
		if c.ID == RootID {
			rootCount++
		}
		if !catalogSet[c.Type] {
			return fmt.Errorf("component %q uses non-catalog type: %s", c.ID, c.Type)
		}
	}
	if rootCount != 1 {
		return fmt.Errorf("spec must contain exactly one %q node, got %d", RootID, rootCount)
	}

	// children 引用完整性
	for _, c := range spec.Components {
		for _, child := range c.Children {
			if !ids[child] {
				return fmt.Errorf("component %q references missing child: %s", c.ID, child)
			}
		}
	}

	// 绑定可解析性：把 DataModel 归一成 interface{} 树后逐个 {path} 解析。
	root, err := toGeneric(spec.DataModel)
	if err != nil {
		return fmt.Errorf("datamodel not serializable: %w", err)
	}
	for _, c := range spec.Components {
		for key, v := range c.Props {
			ptr, ok := bindingPath(v)
			if !ok {
				continue
			}
			if _, ok := ResolvePointer(root, ptr); !ok {
				return fmt.Errorf("component %q prop %q binds unresolved path: %s", c.ID, key, ptr)
			}
		}
	}
	return nil
}

// bindingPath 判断一个 prop 值是否是数据绑定 {"path": "/..."}，是则返回指针字符串。
func bindingPath(v interface{}) (string, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return "", false
	}
	p, ok := m["path"].(string)
	if !ok || p == "" {
		return "", false
	}
	return p, true
}

// toGeneric 把 DataModel 经 JSON 往返成 map/slice/标量树，便于 JSON Pointer 遍历。
func toGeneric(dm *DataModel) (interface{}, error) {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil, err
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolvePointer 按 RFC6901 JSON Pointer 在归一化数据树上取值。
// 空指针 "" 返回整棵树。找不到返回 (nil, false)。
func ResolvePointer(root interface{}, pointer string) (interface{}, bool) {
	if pointer == "" {
		return root, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	cur := root
	for _, raw := range strings.Split(pointer[1:], "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch node := cur.(type) {
		case map[string]interface{}:
			next, ok := node[token]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}
