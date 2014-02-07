package main

import "strings"

// Strips portnumber from remote address and return only the IP-address
func addr2IP(addr string) string {
	i := strings.Index(addr, ":")
	if i == -1 {
		return addr
	}
	return addr[0:i]
}

func sameIP(a, b string) bool {
	return a == b
}