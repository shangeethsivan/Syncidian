package syncengine

import "testing"

func TestAutoMergeTypingContinuation(t *testing.T) {
	got, ok := AutoMerge([]byte("Hello world"), []byte("Hello"))
	if !ok || string(got) != "Hello world" {
		t.Fatalf("prefix local: ok=%v %q", ok, got)
	}
	got, ok = AutoMerge([]byte("Hello"), []byte("Hello world"))
	if !ok || string(got) != "Hello world" {
		t.Fatalf("prefix remote: ok=%v %q", ok, got)
	}
}

func TestAutoMergeSimpleReplacement(t *testing.T) {
	remote := []byte("The cat sat on the mat.\n")
	local := []byte("The dog sat on the mat.\n")
	got, ok := AutoMerge(local, remote)
	if !ok || string(got) != string(local) {
		t.Fatalf("simple replacement: ok=%v %q", ok, got)
	}
}

func TestAutoMergeHugeRewriteOpensConflict(t *testing.T) {
	remote := []byte("# Project\n\nThis is the original design document with a long shared intro that both devices still have.\n")
	local := []byte("# Totally different note\n\n" + string(make([]byte, 600)))
	for i := range local[len("# Totally different note\n\n"):] {
		local[len("# Totally different note\n\n")+i] = 'x'
	}
	if _, ok := AutoMerge(local, remote); ok {
		t.Fatal("huge rewrite should not auto-merge")
	}
}

func TestAutoMergeDeleteVsEdit(t *testing.T) {
	if _, ok := AutoMerge(nil, []byte("still here")); ok {
		t.Fatal("empty local should not auto-merge")
	}
}

func TestAutoMergeBinary(t *testing.T) {
	if _, ok := AutoMerge([]byte{1, 0, 2}, []byte{1, 0, 3}); ok {
		t.Fatal("binary should not auto-merge")
	}
}
