package helper

import (
	"net"
	"testing"
)

func TestNewIPAM(t *testing.T) {
	tests := []struct {
		name    string
		ipRange string
		wantErr bool
	}{
		{"Valid CIDR", "192.168.1.0/24", false},
		{"Invalid CIDR", "192.168.1.0/33", true},
		{"Not CIDR", "192.168.1.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ipam, err := NewIPAM(tt.ipRange)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewIPAM() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && ipam == nil {
				t.Errorf("NewIPAM() returned nil IPAM for valid input")
			}
		})
	}
}

func TestIPAM_AssignIP(t *testing.T) {
	ipam, _ := NewIPAM("192.168.1.0/24")

	// Test assigning multiple IPs
	for i := 0; i < 10; i++ {
		ip, err := ipam.AssignIP()
		if err != nil {
			t.Errorf("AssignIP() error = %v", err)
			return
		}
		if net.ParseIP(ip) == nil {
			t.Errorf("AssignIP() returned invalid IP: %s", ip)
		}
	}

	// Test assigning all available IPs
	for i := 0; i < 244; i++ {
		_, err := ipam.AssignIP()
		if err != nil {
			t.Errorf("AssignIP() error = %v", err)
			return
		}
	}

	// Test assigning IP when all are taken
	_, err := ipam.AssignIP()
	if err == nil {
		t.Errorf("AssignIP() should return error when all IPs are assigned")
	}
}

func TestIPAM_MarkIPAsAssigned(t *testing.T) {
	ipam, _ := NewIPAM("192.168.1.0/24")

	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"Valid IP", "192.168.1.10", false},
		{"Invalid IP", "192.168.1.256", true},
		{"Out of range IP", "192.168.2.1", false}, // This will not return an error as per current implementation
		{"Already assigned IP", "192.168.1.10", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ipam.MarkIPAsAssigned(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarkIPAsAssigned() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPAM_EnsureAssignIPwithMarkIP(t *testing.T) {
	ipam, err := NewIPAM("192.168.1.0/24")
	if err != nil {
		t.Fatalf("Failed to create IPAM: %v", err)
	}

	// IPs to mark as assigned
	preAssignedIPs := []string{
		"192.168.1.10",
		"192.168.1.20",
		"192.168.1.30",
		"192.168.1.40",
		"192.168.1.50",
	}

	// Mark IPs as assigned
	for _, ip := range preAssignedIPs {
		err := ipam.MarkIPAsAssigned(ip)
		if err != nil {
			t.Fatalf("Failed to mark IP %s as assigned: %v", ip, err)
		}
	}

	assignedIPs := make(map[string]bool)
	for _, ip := range preAssignedIPs {
		assignedIPs[ip] = true
	}

	totalIPs := 254 // Total assignable IPs in a /24 network
	remainingIPs := totalIPs - len(preAssignedIPs)

	cidr := ipam.GetCIDR()

	// Assign remaining IPs
	for i := 0; i < remainingIPs; i++ {
		ip, err := ipam.AssignIP()
		if err != nil {
			t.Fatalf("AssignIP() failed on iteration %d: %v", i, err)
		}

		// Check if the IP has already been assigned
		if assignedIPs[ip] {
			t.Errorf("AssignIP() returned duplicate or pre-assigned IP: %s", ip)
		}

		// Mark the IP as assigned
		assignedIPs[ip] = true

		// Validate the IP format
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			t.Errorf("AssignIP() returned invalid IP format: %s", ip)
		}

		// Ensure the IP is within the correct range
		if !cidr.Contains(parsedIP) {
			t.Errorf("AssignIP() returned IP outside of CIDR range: %s", ip)
		}
	}

	// Ensure we've assigned all available IPs
	if len(assignedIPs) != totalIPs {
		t.Errorf("Expected to assign %d unique IPs, but assigned %d", totalIPs, len(assignedIPs))
	}

	// Attempt to assign one more IP, which should fail
	_, err = ipam.AssignIP()
	if err == nil {
		t.Error("AssignIP() should return error when all IPs are assigned")
	}
}
