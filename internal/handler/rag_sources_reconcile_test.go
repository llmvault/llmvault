package handler

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

func TestConfigEntities(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"website urls", `{"urls":["https://a/x","https://a/y"]}`, []string{"https://a/x", "https://a/y"}},
		{"integration scope items", `{"scope":{"resource_type":"team","items":[{"id":"t1"},{"id":"t2"}]}}`, []string{"t1", "t2"}},
		{"dedup + trim", `{"urls":["a"," a ","b"]}`, []string{"a", "b"}},
		{"empty", `{}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := configEntities(json.RawMessage(tc.raw))
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if len(got) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("configEntities = %v, want %v", got, want)
			}
		})
	}
}

func TestSubtractStrings(t *testing.T) {
	old := []string{"a", "b", "c"}
	next := []string{"b", "c", "d"}
	if got := subtractStrings(old, next); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("removed = %v, want [a]", got)
	}
	if got := subtractStrings(next, old); !reflect.DeepEqual(got, []string{"d"}) {
		t.Fatalf("added = %v, want [d]", got)
	}
}
