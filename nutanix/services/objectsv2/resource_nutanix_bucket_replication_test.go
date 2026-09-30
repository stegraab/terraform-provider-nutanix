package objectstoresv2

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

func TestBucketReplicationMutationsAreSerializedPerObjectStore(t *testing.T) {
	const workers = 8
	var active int32
	var maxActive int32
	var waitGroup sync.WaitGroup
	start := make(chan struct{})

	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			unlock, err := lockBucketReplicationMutation(context.Background(), "pc.example.com", "object-store-1")
			if err != nil {
				t.Errorf("lockBucketReplicationMutation() error = %v", err)
				return
			}
			current := atomic.AddInt32(&active, 1)
			for {
				maximum := atomic.LoadInt32(&maxActive)
				if current <= maximum || atomic.CompareAndSwapInt32(&maxActive, maximum, current) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&active, -1)
			unlock()
		}()
	}

	close(start)
	waitGroup.Wait()

	if maxActive != 1 {
		t.Fatalf("maximum concurrent mutations = %d, expected 1", maxActive)
	}
}

func TestBucketReplicationMutationLockPathIsNamespaced(t *testing.T) {
	first := bucketReplicationMutationLockPath("pc-1.example.com", "object-store-1")
	second := bucketReplicationMutationLockPath("pc-2.example.com", "object-store-1")
	third := bucketReplicationMutationLockPath("pc-1.example.com", "object-store-2")

	if first == second || first == third || second == third {
		t.Fatal("expected each Prism Central and object store pair to have a distinct lock path")
	}
	if filepath.Dir(first) != filepath.Clean(filepath.Dir(first)) {
		t.Fatalf("lock path directory %q is not clean", filepath.Dir(first))
	}
}

func TestBucketReplicationMutationsAreSerializedAcrossProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const host = "cross-process.pc.example.com"
	const objectStoreExtID = "cross-process-object-store"
	unlock, err := lockBucketReplicationMutation(ctx, host, objectStoreExtID)
	if err != nil {
		t.Fatalf("lockBucketReplicationMutation() error = %v", err)
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBucketReplicationMutationLockHelper$")
	cmd.Env = append(os.Environ(), "NUTANIX_REPLICATION_LOCK_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting helper process: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatalf("helper did not become ready: %q", scanner.Text())
	}

	acquired := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			acquired <- scanner.Text()
			return
		}
		acquired <- ""
	}()

	select {
	case output := <-acquired:
		t.Fatalf("helper acquired the inter-process lock before release: %q", output)
	case <-time.After(200 * time.Millisecond):
	}

	unlock()
	locked = false
	select {
	case output := <-acquired:
		if output != "acquired" {
			t.Fatalf("unexpected helper output after lock release: %q", output)
		}
	case <-ctx.Done():
		t.Fatal("helper did not acquire the inter-process lock after release")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper process failed: %v", err)
	}
}

func TestBucketReplicationMutationLockHelper(t *testing.T) {
	if os.Getenv("NUTANIX_REPLICATION_LOCK_HELPER") != "1" {
		return
	}

	fmt.Println("ready")
	unlock, err := lockBucketReplicationMutation(context.Background(), "cross-process.pc.example.com", "cross-process-object-store")
	if err != nil {
		t.Fatalf("lockBucketReplicationMutation() error = %v", err)
	}
	defer unlock()
	fmt.Println("acquired")
}

func TestBucketReplicationHasStaleEndpointConflict(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics diag.Diagnostics
		expected    bool
	}{
		{
			name:        "legacy duplicate target message",
			diagnostics: diag.Errorf("Unable to register duplicate endpoint with same target"),
			expected:    true,
		},
		{
			name:        "current duplicate endpoint name message",
			diagnostics: diag.Errorf("Failed to create/update replication rule. Registering endpoint failed with err: Failed to create new endpoint, an endpoint with the same name already exists: 9"),
			expected:    true,
		},
		{
			name: "duplicate endpoint message in detail",
			diagnostics: diag.Diagnostics{
				{
					Severity: diag.Error,
					Summary:  "replication endpoint registration failed",
					Detail:   "an endpoint with the same name already exists: 9",
				},
			},
			expected: true,
		},
		{
			name:        "unrelated replication failure",
			diagnostics: diag.Errorf("target object store is unavailable"),
			expected:    false,
		},
		{
			name: "warning is not a conflict",
			diagnostics: diag.Diagnostics{
				{
					Severity: diag.Warning,
					Summary:  "an endpoint with the same name already exists",
				},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := bucketReplicationHasStaleEndpointConflict(test.diagnostics)
			if actual != test.expected {
				t.Fatalf("bucketReplicationHasStaleEndpointConflict() = %t, expected %t", actual, test.expected)
			}
		})
	}
}

func TestBucketReplicationStaleTargetOSSUUID(t *testing.T) {
	const staleUUID = "11111111-2222-3333-4444-555555555555"
	tests := []struct {
		name        string
		diagnostics diag.Diagnostics
		expected    string
		found       bool
	}{
		{
			name:        "uuid in summary",
			diagnostics: diag.Errorf("Endpoint exists with different targetOssUuid: %s. Unable to register duplicate endpoint with same target", staleUUID),
			expected:    staleUUID,
			found:       true,
		},
		{
			name: "uuid in detail",
			diagnostics: diag.Diagnostics{
				{
					Severity: diag.Error,
					Summary:  "endpoint registration failed",
					Detail:   "different targetOssUuid: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				},
			},
			expected: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			found:    true,
		},
		{
			name:        "name conflict without uuid",
			diagnostics: diag.Errorf("an endpoint with the same name already exists: 9"),
			found:       false,
		},
		{
			name: "warning is ignored",
			diagnostics: diag.Diagnostics{
				{
					Severity: diag.Warning,
					Summary:  "different targetOssUuid: " + staleUUID,
				},
			},
			found: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, found := bucketReplicationStaleTargetOSSUUID(test.diagnostics)
			if found != test.found || actual != test.expected {
				t.Fatalf("bucketReplicationStaleTargetOSSUUID() = %q, %t; expected %q, %t", actual, found, test.expected, test.found)
			}
		})
	}
}

func TestBucketReplicationPayloadWithTargetOSSUUID(t *testing.T) {
	const replication = `{"api_version":"3.0","spec":{"target_oss_uuid":"current-uuid","target_oss_fqdn":"objects.example.com"}}`

	payload, err := bucketReplicationPayloadWithTargetOSSUUID(replication, "stale-uuid")
	if err != nil {
		t.Fatalf("bucketReplicationPayloadWithTargetOSSUUID() error = %v", err)
	}

	var actual map[string]interface{}
	if err := json.Unmarshal(payload, &actual); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	spec := actual["spec"].(map[string]interface{})
	if spec["target_oss_uuid"] != "stale-uuid" {
		t.Fatalf("target_oss_uuid = %q, expected stale-uuid", spec["target_oss_uuid"])
	}
	if spec["target_oss_fqdn"] != "objects.example.com" {
		t.Fatalf("target_oss_fqdn = %q, expected objects.example.com", spec["target_oss_fqdn"])
	}
}

func TestBucketReplicationRemovePayloadAddsTargetPC(t *testing.T) {
	const replication = `{"api_version":"3.0","spec":{"op_mode":"Append","target_oss_uuid":"target-uuid"}}`

	payload, err := bucketReplicationRemovePayload(replication, "pc.example.com")
	if err != nil {
		t.Fatalf("bucketReplicationRemovePayload() error = %v", err)
	}

	var actual map[string]interface{}
	if err := json.Unmarshal(payload, &actual); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	spec := actual["spec"].(map[string]interface{})
	if spec["op_mode"] != "Remove" {
		t.Fatalf("op_mode = %q, expected Remove", spec["op_mode"])
	}
	if spec["target_pc"] != "pc.example.com" {
		t.Fatalf("target_pc = %q, expected pc.example.com", spec["target_pc"])
	}
}

func TestBucketReplicationRemovePayloadPreservesTargetPC(t *testing.T) {
	const replication = `{"api_version":"3.0","spec":{"target_pc":"configured.example.com"}}`

	payload, err := bucketReplicationRemovePayload(replication, "provider.example.com")
	if err != nil {
		t.Fatalf("bucketReplicationRemovePayload() error = %v", err)
	}

	var actual map[string]interface{}
	if err := json.Unmarshal(payload, &actual); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	spec := actual["spec"].(map[string]interface{})
	if spec["target_pc"] != "configured.example.com" {
		t.Fatalf("target_pc = %q, expected configured.example.com", spec["target_pc"])
	}
}

func TestBucketReplicationJSONEquivalentAllowsCanonicalTargetFQDN(t *testing.T) {
	actual := `{
		"api_version":"3.0",
		"spec":{
			"target_oss_uuid":"target-uuid",
			"target_oss_fqdn":"dev-objects-2.prism-central.cluster.local",
			"target_endpoint_list":["10.128.26.66","10.128.26.67"],
			"status":"Enabled"
		}
	}`
	expected := `{
		"api_version":"3.0",
		"spec":{
			"target_oss_uuid":"target-uuid",
			"target_oss_fqdn":"objects-dc2.dev.se-bod.stegra.tech",
			"target_endpoint_list":["10.128.26.66","10.128.26.67"],
			"status":"Enabled"
		}
	}`

	if !bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected canonical and configured target FQDNs to be equivalent for the same target")
	}
}

func TestBucketReplicationJSONEquivalentRejectsDifferentTargetUUID(t *testing.T) {
	actual := `{"spec":{"target_oss_uuid":"actual-uuid","target_oss_fqdn":"internal","target_endpoint_list":["10.0.0.1"]}}`
	expected := `{"spec":{"target_oss_uuid":"expected-uuid","target_oss_fqdn":"external","target_endpoint_list":["10.0.0.1"]}}`

	if bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected different target UUIDs not to be equivalent")
	}
}

func TestBucketReplicationJSONEquivalentRejectsDifferentTargetEndpoints(t *testing.T) {
	actual := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"internal","target_endpoint_list":["10.0.0.1"]}}`
	expected := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"external","target_endpoint_list":["10.0.0.2"]}}`

	if bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected different target endpoint lists not to be equivalent")
	}
}

func TestBucketReplicationJSONEquivalentStillDetectsOtherChanges(t *testing.T) {
	actual := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"internal","target_endpoint_list":["10.0.0.1"],"status":"Enabled"}}`
	expected := `{"spec":{"target_oss_uuid":"target-uuid","target_oss_fqdn":"external","target_endpoint_list":["10.0.0.1"],"status":"Disabled"}}`

	if bucketReplicationJSONEquivalent(actual, expected) {
		t.Fatal("expected non-FQDN replication changes not to be equivalent")
	}
}
