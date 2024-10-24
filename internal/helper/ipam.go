package helper

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/exp/rand"
)

// Custom error types
type IPAMWarning struct {
	Message string
}

func (e *IPAMWarning) Error() string {
	return e.Message
}

type IPAMError struct {
	Message string
}

func (e *IPAMError) Error() string {
	return e.Message
}

// IPAM is the main IP Address Management system
type IPAM struct {
	cidr        *net.IPNet
	assignedIPs map[string]bool
	mu          sync.Mutex
}

// NewIPAM creates a new IPAM instance
func NewIPAM(ipRange string) (*IPAM, error) {
	_, ipNet, err := net.ParseCIDR(ipRange)
	if err != nil {
		return nil, err
	}

	return &IPAM{
		cidr:        ipNet,
		assignedIPs: make(map[string]bool),
	}, nil
}

// Safe increment function
func incrementIP(ip net.IP) net.IP {
	newIP := make(net.IP, len(ip))
	copy(newIP, ip)
	for j := len(newIP) - 1; j >= 0; j-- {
		newIP[j]++
		if newIP[j] > 0 {
			break
		}
	}
	return newIP
}

// Safe decrement function
func decrementIP(ip net.IP) net.IP {
	newIP := make(net.IP, len(ip))
	copy(newIP, ip)
	for j := len(newIP) - 1; j >= 0; j-- {
		newIP[j]--
		if newIP[j] < 255 {
			break
		}
	}
	return newIP
}

// AssignIP assigns a random IP address to a node
func (ipam *IPAM) AssignIP() (string, error) {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	// Calculate start and end IPs from CIDR
	startIP := ipam.cidr.IP.To4()
	endIP := make(net.IP, len(startIP))
	for i := range endIP {
		endIP[i] = startIP[i] | ^ipam.cidr.Mask[i]
	}
	endIP = decrementIP(endIP) // Remove broadcast address

	// Debug: Print current state
	// fmt.Println("AssignIP called")
	// fmt.Println("startIP:", startIP)
	// fmt.Println("endIP:", endIP)

	// Seed the random number generator
	rand.Seed(uint64(time.Now().UnixNano()))

	// Collect all possible IPs within the range
	var possibleIPs []string
	for ip := startIP; !ip.Equal(endIP); ip = incrementIP(ip) {
		ipStr := ip.String()
		if !ipam.assignedIPs[ipStr] {
			possibleIPs = append(possibleIPs, ipStr)
		}
	}

	// Check if there are any possible IPs left
	if len(possibleIPs) == 0 {
		// Log the ipam object
		// fmt.Println("no ips available ipam object", ipam)
		return "", fmt.Errorf("no available IP addresses")
	}

	// Select a random IP from the possible IPs
	index := rand.Intn(len(possibleIPs))
	ip := possibleIPs[index]
	ipam.assignedIPs[ip] = true
	return ip, nil
}

// ReleaseIP releases an IP address
// func (ipam *IPAM) ReleaseIP(ip string) error {
// 	ipam.mu.Lock()
// 	defer ipam.mu.Unlock()

// 	if !ipam.assignedIPs[ip] {
// 		return fmt.Errorf("IP address %s is not assigned", ip)
// 	}

// 	delete(ipam.assignedIPs, ip)
// 	return nil
// }

// MarkIPAsAssigned marks an IP address as assigned
func (ipam *IPAM) MarkIPAsAssigned(ip string) error {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	parsedIP := net.ParseIP(ip).To4()
	if parsedIP == nil {
		return &IPAMError{Message: fmt.Sprintf("invalid IP address: %s", ip)}
	}

	// Check if the IP is within the valid range
	if !ipam.cidr.Contains(parsedIP) {
		// return &IPAMWarning{Message: fmt.Sprintf("IP address %s is out of range", ip)}
		//suppress warning. Not required
		return nil
	}

	// Check if the IP is already assigned
	if ipam.assignedIPs[ip] {
		return &IPAMWarning{Message: fmt.Sprintf("IP address %s is already assigned", ip)}
	}

	// Mark the IP as assigned
	ipam.assignedIPs[ip] = true

	return nil
}

// GetCIDR returns the CIDR of the IPAM
func (ipam *IPAM) GetCIDR() *net.IPNet {
	return ipam.cidr
}
