package lens

// This file implements the per-field ownership model used by the resource
// lenses. A field is "owned" by the operator when the renderer-produced spec
// (the lens's desired state) has it set; otherwise the field is considered
// externally managed and the lens neither projects it nor overwrites it.

// projectOwnedPtr returns a deep copy of src when owned is non-nil; otherwise
// nil. Use from projection paths so that externally-set fields do not show
// up as diffs in EqualResource.
func projectOwnedPtr[T any, PT interface {
	*T
	DeepCopyInto(*T)
}](src, owned PT) PT {
	if owned == nil || src == nil {
		return nil
	}
	var out T
	src.DeepCopyInto(&out)
	return &out
}

// projectOwnedSlice returns a deep copy of src when owned is non-nil;
// otherwise nil.
func projectOwnedSlice[T any, PT interface {
	*T
	DeepCopyInto(*T)
}](src, owned []T) []T {
	if owned == nil {
		return nil
	}
	return cloneSlice[T, PT](src)
}

// projectOwnedScalar returns a copy of src when owned is non-nil; otherwise
// nil. Use for *int32/*int64/*bool/*string fields with no DeepCopy method.
func projectOwnedScalar[T any](src, owned *T) *T {
	if owned == nil || src == nil {
		return nil
	}
	v := *src
	return &v
}

// applyOwnedPtr sets *cur to a deep copy of desired when desired is non-nil;
// otherwise leaves *cur alone (preserves externally-set values).
func applyOwnedPtr[T any, PT interface {
	*T
	DeepCopyInto(*T)
}](cur **T, desired PT) {
	if desired == nil {
		return
	}
	var out T
	desired.DeepCopyInto(&out)
	*cur = &out
}

// applyOwnedSlice sets *cur to a deep copy of desired when desired is
// non-nil; otherwise leaves *cur alone. A non-nil empty desired clears *cur.
func applyOwnedSlice[T any, PT interface {
	*T
	DeepCopyInto(*T)
}](cur *[]T, desired []T) {
	if desired == nil {
		return
	}
	*cur = cloneSlice[T, PT](desired)
}

// applyOwnedScalar sets *cur to a copy of desired when desired is non-nil;
// otherwise leaves *cur alone.
func applyOwnedScalar[T any](cur **T, desired *T) {
	if desired == nil {
		return
	}
	v := *desired
	*cur = &v
}

// cloneSlice returns a fresh slice with each element deep-copied. Returns
// nil for an empty input.
func cloneSlice[T any, PT interface {
	*T
	DeepCopyInto(*T)
}](src []T) []T {
	if len(src) == 0 {
		return nil
	}
	out := make([]T, len(src))
	for i := range src {
		PT(&src[i]).DeepCopyInto(&out[i])
	}
	return out
}
