package store

import (
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	// "github.com/go-logr/zapr"
	// "go.uber.org/zap"
	// "go.uber.org/zap/zapcore"
	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// // "github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
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
	found := s.Upsert(&o1)
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
	found = s.Upsert(&o1)
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
	found = s.Upsert(&o2)
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
	found = s.Upsert(&o1)
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
	found = s.Upsert(&o1)
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
	found = s.Upsert(&o2)
	assert.True(t, found, "upsert 3")
	assert.Equal(t, 2, s.Len(), "len 3")
	found = s.Upsert(&o3)
	assert.True(t, found, "upsert 3")
	assert.Equal(t, 3, s.Len(), "len 3")
	found = s.Upsert(&o3)
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
