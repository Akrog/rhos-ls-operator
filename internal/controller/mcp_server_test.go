/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"testing"
)

func TestBuildLCoreMCPServersConfig_WithOpenStack(t *testing.T) {
	servers := buildLCoreMCPServersConfig(true)

	if len(servers) != 2 {
		t.Errorf("expected 2 MCP servers, got %d", len(servers))
	}

	// Verify first server is OpenShift MCP
	first := servers[0].(map[string]interface{})
	if first["name"] != "rhos-ocp-tools" {
		t.Errorf("expected first server name 'rhos-ocp-tools', got '%s'", first["name"])
	}
	expectedURL := fmt.Sprintf("http://%s:%d/openshift/", MCPServiceName, MCPServerPort)
	if first["url"] != expectedURL {
		t.Errorf("expected first server url '%s', got '%s'", expectedURL, first["url"])
	}
	authHeaders := first["authorization_headers"].(map[string]interface{})
	if authHeaders["OCP_TOKEN"] != "kubernetes" {
		t.Errorf("expected OCP_TOKEN authorization_header 'kubernetes', got '%s'", authHeaders["OCP_TOKEN"])
	}

	// Verify second server is OpenStack MCP
	second := servers[1].(map[string]interface{})
	if second["name"] != "rhos-osp-tools" {
		t.Errorf("expected second server name 'rhos-osp-tools', got '%s'", second["name"])
	}
	expectedURL = fmt.Sprintf("http://%s:%d/openstack/", MCPServiceName, MCPServerPort)
	if second["url"] != expectedURL {
		t.Errorf("expected second server url '%s', got '%s'", expectedURL, second["url"])
	}
}

func TestBuildLCoreMCPServersConfig_WithoutOpenStack(t *testing.T) {
	servers := buildLCoreMCPServersConfig(false)

	if len(servers) != 1 {
		t.Errorf("expected 1 MCP server, got %d", len(servers))
	}

	// Verify only OpenShift MCP is present
	first := servers[0].(map[string]interface{})
	if first["name"] != "rhos-ocp-tools" {
		t.Errorf("expected first server name 'rhos-ocp-tools', got '%s'", first["name"])
	}
}

func TestGetMCPServerURL(t *testing.T) {
	expected := fmt.Sprintf("http://%s:%d", MCPServiceName, MCPServerPort)
	actual := GetMCPServerURL()
	if actual != expected {
		t.Errorf("expected '%s', got '%s'", expected, actual)
	}
}
