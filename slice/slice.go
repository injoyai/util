// Package slice 提供泛型切片操作；排序、复制等直接使用标准库 slices。
package slice

// Get 支持负数索引，-1 表示最后一个元素。
func Get[T any](s []T, index int) (T, bool) {
	if index < 0 {
		index += len(s)
	}
	if index >= 0 && index < len(s) {
		return s[index], true
	}
	var zero T
	return zero, false
}

// Unique 去重且保留首次出现顺序，返回独立切片。
func Unique[T comparable](s []T) []T {
	result := make([]T, 0, len(s))
	seen := make(map[T]struct{}, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Common 返回所有切片的交集，按第一个切片顺序去重，返回独立切片。
func Common[T comparable](lists ...[]T) []T {
	if len(lists) == 0 {
		return nil
	}
	result := Unique(lists[0])
	for _, list := range lists[1:] {
		set := make(map[T]struct{}, len(list))
		for _, v := range list {
			set[v] = struct{}{}
		}
		out := result[:0]
		for _, v := range result {
			if _, ok := set[v]; ok {
				out = append(out, v)
			}
		}
		result = out
	}
	return result
}

// Limit 返回 offset 起最多 size 个元素的视图，与输入共享存储。
// 负 offset 按 0 处理，size<=0 或越界返回空切片。
func Limit[T any](s []T, size, offset int) []T {
	if offset < 0 {
		offset = 0
	}
	if size <= 0 || offset >= len(s) {
		return s[:0]
	}
	if size > len(s)-offset {
		size = len(s) - offset
	}
	return s[offset : offset+size]
}
