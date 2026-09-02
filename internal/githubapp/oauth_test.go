package githubapp

import (
	"reflect"
	"testing"
)

func TestMergeEmails(t *testing.T) {
	got := mergeEmails("Ada@Example.com", []string{"ada@example.com", "work@example.com", ""})
	want := []string{"Ada@Example.com", "work@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := mergeEmails("", nil); got != nil && len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
}
