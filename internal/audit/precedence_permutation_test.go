package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ninoamine/istio-route/internal/source"
)

const precedencePermutationsPerFixture = 200

func TestPrecedenceAuditIsOrderIndependent(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/manifests/gateway-audit-precedence-*.yaml")
	if err != nil {
		t.Fatalf("glob precedence fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no gateway-audit-precedence-*.yaml fixtures found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			snapshot, err := (&source.FilesSource{Files: []string{path}}).Snapshot(context.Background())
			if err != nil {
				t.Fatalf("load fixture: %v", err)
			}
			want := serializedPrecedenceAudit(t, snapshot)

			reversed := snapshot
			reversed.Resources = slices.Clone(snapshot.Resources)
			slices.Reverse(reversed.Resources)
			if got := serializedPrecedenceAudit(t, reversed); !bytes.Equal(got, want) {
				t.Fatalf("reversed input order changed audit output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}

			rng := rand.New(rand.NewPCG(1, 2))
			for i := range precedencePermutationsPerFixture {
				shuffled := snapshot
				shuffled.Resources = slices.Clone(snapshot.Resources)
				rng.Shuffle(len(shuffled.Resources), func(a, b int) {
					shuffled.Resources[a], shuffled.Resources[b] = shuffled.Resources[b], shuffled.Resources[a]
				})
				if got := serializedPrecedenceAudit(t, shuffled); !bytes.Equal(got, want) {
					t.Fatalf("permutation %d changed audit output:\n--- got ---\n%s\n--- want ---\n%s", i, got, want)
				}
			}
		})
	}
}

func serializedPrecedenceAudit(t *testing.T, snapshot source.Snapshot) []byte {
	t.Helper()
	result, _, err := Analyze(context.Background(), Request{
		GatewayName: "edge", Namespace: "infra", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("analyze precedence fixture: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode audit result: %v", err)
	}
	return encoded
}
