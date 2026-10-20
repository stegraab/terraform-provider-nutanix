package objectstoresv2

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestBucketReplicationReusesSharedEndpointAcrossBuckets(t *testing.T) {
	for _, test := range []struct {
		name                                                                             string
		failList, failRead, noMatch, ambiguous, wrongPC, noName, retryFailure, unrelated bool
	}{
		{name: "reuse for multiple new buckets"},
		{name: "listing denied", failList: true},
		{name: "rule read denied", failRead: true},
		{name: "different target UUID", noMatch: true},
		{name: "conflicting names", ambiguous: true},
		{name: "different target Prism", wrongPC: true},
		{name: "missing registered name", noName: true},
		{name: "retry fails", retryFailure: true},
		{name: "unrelated error does not discover", unrelated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			puts, lists, offsetReads := 0, 0, 0
			wanted := map[string]interface{}{
				"api_version": "3.0", "metadata": map[string]interface{}{"source_bucket_name": "new-a"},
				"spec": map[string]interface{}{
					"target_oss_uuid": "target-uuid", "target_oss_fqdn": "alias.example.com", "target_pc": "pc.example.com",
					"target_endpoint_list": []interface{}{"192.0.2.1"}, "target_bucket_name": "new-a", "op_mode": "Append",
					"filters": []interface{}{map[string]interface{}{"id": "all", "prefix": "", "replicate_delete_markers": true}},
				},
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					puts++
					var actual map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					spec := actual["spec"].(map[string]interface{})
					if spec["op_mode"] != "Append" || (r.URL.Path != "/oss/api/nutanix/v3/objectstore_proxy/source/buckets/new-a/replication_rule" && r.URL.Path != "/oss/api/nutanix/v3/objectstore_proxy/source/buckets/new-b/replication_rule") {
						t.Errorf("unexpected mutation: %s %s %v", r.Method, r.URL.Path, actual)
						w.WriteHeader(400)
						return
					}
					if spec["target_oss_fqdn"] == "registered.example.com" {
						expected := wanted["spec"].(map[string]interface{})
						for k, v := range expected {
							if k != "target_oss_fqdn" && !reflect.DeepEqual(v, spec[k]) {
								t.Errorf("field %s changed: %v != %v", k, spec[k], v)
							}
						}
						if test.retryFailure {
							w.WriteHeader(503)
							fmt.Fprint(w, `{"message":"target unavailable"}`)
							return
						}
						w.WriteHeader(202)
						fmt.Fprint(w, `{}`)
						return
					}
					if test.unrelated {
						w.WriteHeader(403)
						fmt.Fprint(w, `{"message":"permission denied"}`)
						return
					}
					w.WriteHeader(500)
					fmt.Fprint(w, `{"message":"an endpoint with the same name already exists: 9"}`)
					return
				}
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/groups") {
					lists++
					if test.failList {
						w.WriteHeader(403)
						fmt.Fprint(w, `{}`)
						return
					}
					var query struct {
						Offset int `json:"group_member_offset"`
					}
					if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
						t.Error(err)
					}
					// Deliberately return short pages to exercise offset advancement.
					if query.Offset == 0 {
						fmt.Fprint(w, `{"filtered_entity_count":3,"group_results":[{"entity_results":[{"entity_id":"unreplicated"},{"entity_id":"existing-a"}]}]}`)
					} else if query.Offset == 2 {
						offsetReads++
						fmt.Fprint(w, `{"filtered_entity_count":3,"group_results":[{"entity_results":[{"entity_id":"existing-b"}]}]}`)
					} else {
						t.Errorf("unexpected offset %d", query.Offset)
						w.WriteHeader(400)
					}
					return
				}
				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/replication_rule") {
					if strings.Contains(r.URL.Path, "/unreplicated/") {
						w.WriteHeader(404)
						fmt.Fprint(w, `{}`)
						return
					}
					if test.failRead {
						w.WriteHeader(403)
						fmt.Fprint(w, `{}`)
						return
					}
					target, name, pc := "target-uuid", "registered.example.com", "pc.example.com"
					if test.noMatch {
						target = "other-target-uuid"
					}
					if test.ambiguous && strings.Contains(r.URL.Path, "/existing-b/") {
						name = "conflicting.example.com"
					}
					if test.wrongPC {
						pc = "different-pc.example.com"
					}
					if test.noName {
						name = ""
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"specs": []interface{}{map[string]interface{}{"spec": map[string]interface{}{"target_oss_uuid": target, "target_oss_fqdn": name, "target_pc": pc}}}})
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(400)
			}))
			defer server.Close()
			parsed, _ := url.Parse(server.URL)
			host, portString, _ := net.SplitHostPort(parsed.Host)
			port, _ := strconv.Atoi(portString)
			cfg := &objectStoreProxyConfig{Scheme: "http", Host: host, Port: port}
			raw, _ := json.Marshal(wanted)
			result := applyBucketReplication(context.Background(), cfg, "source", "new-a", string(raw))
			success := test.name == "reuse for multiple new buckets"
			if result.HasError() == success {
				t.Fatalf("unexpected result: %v", result)
			}
			if success {
				if puts != 2 || lists != 2 || offsetReads != 1 {
					t.Fatalf("unexpected request counts: puts=%d lists=%d final-pages=%d", puts, lists, offsetReads)
				}
				// Another bucket on the same source must reuse exactly the same identity.
				wanted["metadata"].(map[string]interface{})["source_bucket_name"] = "new-b"
				wanted["spec"].(map[string]interface{})["target_bucket_name"] = "new-b"
				raw, _ = json.Marshal(wanted)
				if result = applyBucketReplication(context.Background(), cfg, "source", "new-b", string(raw)); result.HasError() {
					t.Fatal(result)
				}
				if puts != 4 {
					t.Fatalf("expected two append attempts per bucket, got %d", puts)
				}
				if wanted["spec"].(map[string]interface{})["target_oss_fqdn"] != "alias.example.com" {
					t.Fatal("input configuration changed")
				}
			} else if test.retryFailure {
				if puts != 2 {
					t.Fatalf("retry must be bounded, got %d attempts", puts)
				}
			} else if puts != 1 {
				t.Fatalf("failed discovery must not mutate or retry, got %d attempts", puts)
			}
			if test.unrelated && lists != 0 {
				t.Fatal("unrelated errors must not trigger discovery")
			}
		})
	}
}

func TestBucketReplicationEndpointDiscoveryRejectsIncompleteListing(t *testing.T) {
	for _, response := range []string{
		`{`, `{}`, `{"filtered_entity_count":1,"error_message_list":["denied"]}`,
		`{"filtered_entity_count":1,"group_results":[]}`,
		`{"filtered_entity_count":1,"group_results":[{"entity_results":[{"entity_id":""}]}]}`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
			defer server.Close()
			parsed, _ := url.Parse(server.URL)
			host, ps, _ := net.SplitHostPort(parsed.Host)
			port, _ := strconv.Atoi(ps)
			_, err := bucketReplicationPayloadWithRegisteredTarget(context.Background(), &objectStoreProxyConfig{Scheme: "http", Host: host, Port: port}, "source", []byte(`{"spec":{"target_oss_uuid":"target"}}`))
			if err == nil {
				t.Fatal("incomplete listing was accepted")
			}
		})
	}
}

func TestBucketReplicationDoesNotDiscoverAfterSuccessfulAppend(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPut {
			t.Errorf("unexpected %s request", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	host, ps, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(ps)
	result := applyBucketReplication(context.Background(), &objectStoreProxyConfig{Scheme: "http", Host: host, Port: port}, "source", "bucket", `{"spec":{"target_oss_uuid":"target","target_oss_fqdn":"alias.example.com"}}`)
	if result.HasError() || requests != 1 {
		t.Fatalf("unexpected result or extra requests: %v, %d", result, requests)
	}
}
