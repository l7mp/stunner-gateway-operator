package store

import (
	// "fmt"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// for debugging
// var testerLogLevel = zapcore.Level(-4)

// info
// var testerLogLevel = zapcore.DebugLevel

// var testerLogLevel = zapcore.ErrorLevel

var (
	o1 = corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "s1"}}
	o2 = corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "s2"}}
	o3 = corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "s3"}}
)

func keys(os []client.Object) []string {
	ret := []string{}
	for _, o := range os {
		ret = append(ret, GetObjectKey(o))
	}
	return ret
}

func TestStore(t *testing.T) {
	// zc := zap.NewProductionConfig()
	// zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
	// z, err := zc.Build()
	// assert.NoError(t, err, "logger created")
	// log := zapr.NewLogger(z)

	// new
	s := NewStore() // log.WithName("store"))
	assert.NotNil(t, s, "new")

	// upsert
	found := s.UpsertIfChanged(&o1)
	assert.True(t, found, "upsert")
	assert.Equal(t, 1, s.Len(), "len")

	// objects
	os := s.Objects()
	assert.Len(t, os, 1, "objects")
	ks := keys(os)
	assert.Contains(t, ks, "default/s1", "objects content")

	// get
	o := s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "get content")

	// re-upsert
	found = s.UpsertIfChanged(&o1)
	assert.False(t, found, "re-upsert")
	assert.Equal(t, 1, s.Len(), "re-len")

	// re-objects
	os = s.Objects()
	assert.Len(t, os, 1, "re-objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "re-objects content")

	// re-get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "re-get content")

	// upsert 2
	found = s.UpsertIfChanged(&o2)
	assert.True(t, found, "upsert 2")
	assert.Equal(t, 2, s.Len(), "len 2")

	// objects
	os = s.Objects()
	assert.Len(t, os, 2, "objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "objects content 2")
	assert.Contains(t, ks, "default/s2", "objects content 2")

	// get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "get content 2: s1")
	o = s.Get(GetNameFromKey("default/s2"))
	assert.Equal(t, "default/s2", GetObjectKey(o), "get content 2: s2")

	// re-upsert
	found = s.UpsertIfChanged(&o1)
	assert.False(t, found, "re-upsert 2")
	assert.Equal(t, 2, s.Len(), "re-len 2")

	// re-objects
	os = s.Objects()
	assert.Len(t, os, 2, "re-objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "re-objects content 2")
	assert.Contains(t, ks, "default/s2", "re-objects content 2")

	// re-get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "re-get content 2: s1")
	o = s.Get(GetNameFromKey("default/s2"))
	assert.Equal(t, "default/s2", GetObjectKey(o), "re-get content 2: s2")

	// remove
	s.Remove(GetNameFromKey("default/s2"))

	// objects
	os = s.Objects()
	assert.Len(t, os, 1, "objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "objects content")

	// get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "get content")

	// re-remove
	s.Remove(GetNameFromKey("default/s2"))

	// objects
	os = s.Objects()
	assert.Len(t, os, 1, "objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "objects content")

	// get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "get content")

	// re-upsert
	found = s.UpsertIfChanged(&o1)
	assert.False(t, found, "re-upsert")
	assert.Equal(t, 1, s.Len(), "re-len")

	// re-objects
	os = s.Objects()
	assert.Len(t, os, 1, "re-objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "re-objects content")

	// re-get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "re-get content")

	// upsert 3
	found = s.UpsertIfChanged(&o2)
	assert.True(t, found, "upsert 3")
	assert.Equal(t, 2, s.Len(), "len 3")
	found = s.UpsertIfChanged(&o3)
	assert.True(t, found, "upsert 3")
	assert.Equal(t, 3, s.Len(), "len 3")
	found = s.UpsertIfChanged(&o3)
	assert.False(t, found, "upsert 3")
	assert.Equal(t, 3, s.Len(), "len 3")

	// objects
	os = s.Objects()
	assert.Len(t, os, 3, "objects")
	ks = keys(os)
	assert.Contains(t, ks, "default/s1", "objects content 3")
	assert.Contains(t, ks, "default/s2", "objects content 3")
	assert.Contains(t, ks, "default/s3", "objects content 3")

	// get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Equal(t, "default/s1", GetObjectKey(o), "get content 3: s1")
	o = s.Get(GetNameFromKey("default/s2"))
	assert.Equal(t, "default/s2", GetObjectKey(o), "get content 3: s2")
	o = s.Get(GetNameFromKey("default/s3"))
	assert.Equal(t, "default/s3", GetObjectKey(o), "get content 3: s3")

	// remove all
	s.Remove(GetNameFromKey("default/s1"))
	s.Remove(GetNameFromKey("default/s2"))
	s.Remove(GetNameFromKey("default/s3"))

	// objects
	os = s.Objects()
	assert.Len(t, os, 0, "objects")

	// get
	o = s.Get(GetNameFromKey("default/s1"))
	assert.Nil(t, o, "get fails")
	o = s.Get(GetNameFromKey("default/s2"))
	assert.Nil(t, o, "get fails")
	o = s.Get(GetNameFromKey("default/s3"))
	assert.Nil(t, o, "get fails")
}

func TestFilterLabels(t *testing.T) {
	log := logr.Discard()

	t.Run("empty input", func(t *testing.T) {
		out := FilterLabels(nil, []string{"applyset.kubernetes.io/part-of"}, log)
		assert.Empty(t, out)
	})

	t.Run("empty filter", func(t *testing.T) {
		in := map[string]string{"a": "1", "b": "2"}
		out := FilterLabels(in, nil, log)
		assert.Equal(t, in, out)
	})

	t.Run("filter hits", func(t *testing.T) {
		in := map[string]string{
			"applyset.kubernetes.io/part-of": "applyset-main",
			"app.kubernetes.io/managed-by":   "Helm",
			"keep-me":                        "yes",
		}
		out := FilterLabels(in, []string{
			"applyset.kubernetes.io/part-of",
			"app.kubernetes.io/managed-by",
		}, log)
		assert.Len(t, out, 1)
		assert.Equal(t, "yes", out["keep-me"])
		_, ok := out["applyset.kubernetes.io/part-of"]
		assert.False(t, ok, "applyset key stripped")
		_, ok = out["app.kubernetes.io/managed-by"]
		assert.False(t, ok, "managed-by key stripped")
	})

	t.Run("filter misses", func(t *testing.T) {
		in := map[string]string{"a": "1", "b": "2"}
		out := FilterLabels(in, []string{"not-present"}, log)
		assert.Equal(t, in, out)
	})

	t.Run("input not mutated", func(t *testing.T) {
		in := map[string]string{
			"applyset.kubernetes.io/part-of": "x",
			"keep-me":                        "yes",
		}
		_ = FilterLabels(in, []string{"applyset.kubernetes.io/part-of"}, log)
		_, ok := in["applyset.kubernetes.io/part-of"]
		assert.True(t, ok, "input map preserved")
		assert.Len(t, in, 2)
	})
}
