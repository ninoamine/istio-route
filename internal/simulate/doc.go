// Package simulate is a pure, in-memory, read-only simulation domain for
// HTTPRoute removal impact. It performs no I/O: callers supply the snapshot
// and the package never reads from or writes to the cluster or the filesystem.
package simulate
