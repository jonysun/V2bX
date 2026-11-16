package v2bxusers

func Upsert[T any](current, updates []T, name func(T) string) []T {
	result := append([]T(nil), current...)
	indices := make(map[string]int, len(result))
	for i, user := range result {
		indices[name(user)] = i
	}
	for _, user := range updates {
		key := name(user)
		if index, found := indices[key]; found {
			result[index] = user
			continue
		}
		indices[key] = len(result)
		result = append(result, user)
	}
	return result
}

func Remove[T any](current []T, names []string, name func(T) string) []T {
	removed := make(map[string]struct{}, len(names))
	for _, value := range names {
		removed[value] = struct{}{}
	}
	result := make([]T, 0, len(current))
	for _, user := range current {
		if _, found := removed[name(user)]; !found {
			result = append(result, user)
		}
	}
	return result
}
