package cidrs

import (
	"fmt"
	"log"
	"net"
)

// Cidrs contains a map of desired inventory groups and the subnets associated with them
type Cidrs map[string]*net.IPNet

// parseCIDRs compares an IP address to a range of subnets.  If the address is in the subnet, the name of the subnet
// is appended to a subnets list and returned
func (c Cidrs) ParseCIDRs(ipAddr string) (memberOf []string) {
	cidrString := fmt.Sprintf("%s/32", ipAddr)
	ip, _, err := net.ParseCIDR(cidrString)
	if err != nil {
		log.Print("Invalid CIDR address")
	}

	// Iterate through each defined subnet and test if the address is a member of it.
	for name, cidr := range c {
		if cidr.Contains(ip) {
			memberOf = append(memberOf, name)
		}
	}
	return
}

// AddCIDR adds a CIDR name and subnet to the members list.
func (c Cidrs) AddCIDR(name, subnet string) {
	_, cidr, err := net.ParseCIDR(subnet)
	if err != nil {
		log.Printf("Invalid subnet: %v", err)
	}
	c[name] = cidr
}

// AddCIDRMap is a helper function for adding multiple subnets to the members list.
func (c Cidrs) AddCIDRMap(cidrMap map[string]string) {
	for name, cidr := range cidrMap {
		c.AddCIDR(name, cidr)
	}
}
