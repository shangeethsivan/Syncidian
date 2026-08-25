package syncengine

import "bytes"

// AutoMerge combines two diverged versions when the change is a simple
// replacement or typing continuation. Large rewrites return ok=false so the
// conflict UI can open. local is the pushing device; remote is the server copy.
func AutoMerge(local, remote []byte) (merged []byte, ok bool) {
	if bytes.Equal(local, remote) {
		return local, true
	}
	if len(local) == 0 || len(remote) == 0 {
		return nil, false
	}
	if looksBinary(local) || looksBinary(remote) {
		return nil, false
	}
	if bytes.HasPrefix(local, remote) || bytes.HasSuffix(local, remote) {
		return local, true
	}
	if bytes.HasPrefix(remote, local) || bytes.HasSuffix(remote, local) {
		return remote, true
	}

	pre, suf := commonAffix(local, remote)
	if pre+suf == 0 {
		return nil, false
	}
	localMid := local[pre : len(local)-suf]
	remoteMid := remote[pre : len(remote)-suf]
	maxLen := len(local)
	if len(remote) > maxLen {
		maxLen = len(remote)
	}
	change := len(localMid)
	if len(remoteMid) > change {
		change = len(remoteMid)
	}
	unchanged := pre + suf
	if change > 4096 {
		return nil, false
	}
	if unchanged*2 < maxLen && change > 512 {
		return nil, false
	}
	if unchanged >= change {
		return local, true
	}
	return nil, false
}

func looksBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(b[:n], 0) >= 0
}

func commonAffix(a, b []byte) (prefix, suffix int) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for prefix < n && a[prefix] == b[prefix] {
		prefix++
	}
	maxSuf := n - prefix
	for suffix < maxSuf && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	return prefix, suffix
}
