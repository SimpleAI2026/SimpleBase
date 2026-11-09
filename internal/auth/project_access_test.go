package auth

import "testing"

func TestCheckProjectAccess(t *testing.T) {
	cases := []struct {
		name      string
		principal Principal
		project   string
		tenant    string
		system    bool
		want      bool
	}{
		{"key own", Principal{TenantID: "t", ProjectIDs: map[string]struct{}{"a": {}}}, "a", "t", false, true},
		{"key other", Principal{TenantID: "t", ProjectIDs: map[string]struct{}{"a": {}}}, "b", "t", false, false},
		{"key system", Principal{TenantID: "t", ProjectIDs: map[string]struct{}{"a": {}}}, "sb-admin", "t", true, false},
		{"admin key other", Principal{TenantID: "t", Permissions: map[Permission]struct{}{ProjectAdmin: {}}}, "b", "t", false, true},
		{"admin key system", Principal{TenantID: "t", Permissions: map[Permission]struct{}{ProjectAdmin: {}}}, "sb-admin", "t", true, true},
		{"user own", Principal{TenantID: "t", Role: RoleUser, ProjectIDs: map[string]struct{}{"a": {}}, Permissions: map[Permission]struct{}{ProjectAdmin: {}}}, "a", "t", false, true},
		{"user other despite admin permission", Principal{TenantID: "t", Role: RoleUser, ProjectIDs: map[string]struct{}{"a": {}}, Permissions: map[Permission]struct{}{ProjectAdmin: {}}}, "b", "t", false, false},
		{"user system", Principal{TenantID: "t", Role: RoleUser, Permissions: map[Permission]struct{}{ProjectAdmin: {}}}, "sb-admin", "t", true, false},
		{"admin", Principal{TenantID: "t", Role: RoleAdmin}, "b", "t", false, true},
		{"super system", Principal{TenantID: "t", Role: RoleSuperAdmin}, "sb-admin", "t", true, true},
		{"cross tenant", Principal{TenantID: "t", Role: RoleSuperAdmin}, "b", "other", false, false},
		{"empty tenant", Principal{Role: RoleSuperAdmin}, "b", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckProjectAccess(tc.principal, tc.project, tc.tenant, tc.system); got != tc.want {
				t.Fatalf("CheckProjectAccess() = %v, want %v", got, tc.want)
			}
		})
	}
}
