package cidrs

import (
	"testing"
)

func contains(sl []string, st string) bool {
	for _, sls := range sl {
		if sls == st {
			return true
		}
	}
	return false
}

func TestCIDR(t *testing.T) {
	c := make(Cidrs)
	c.AddCIDRMap(map[string]string{
		"test1": "192.168.0.0/24",
		"test2": "192.168.1.0/24",
	})
	testAddr := "192.168.0.5"
	memberOf := c.ParseCIDRs(testAddr)
	if !contains(memberOf, "test1") {
		t.Fatalf("%s should be a member of test1", testAddr)
	}
	if contains(memberOf, "test2") {
		t.Fatalf("%s should not be a member of test2", testAddr)
	}

	testAddr = "192.168.1.254"
	memberOf = c.ParseCIDRs(testAddr)
	if !contains(memberOf, "test2") {
		t.Fatalf("%s should be a member of test2", testAddr)
	}
	if contains(memberOf, "test1") {
		t.Fatalf("%s should not be a member of test1", testAddr)
	}
}
