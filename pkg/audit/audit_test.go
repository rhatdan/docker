package audit

import (
	"io/ioutil"
	"net"
	"testing"
)

func TestLibAudit(t *testing.T) {
	if audit.AuditValueNeedsEncoding("test") {
		t.Fatal("Expected false for AuditValueNeedsEncoding , received true: ")
	}
	if !audit.AuditValueNeedsEncoding("test test") {
		t.Fatal("Expected true for AuditValueNeedsEncoding , received false: ")
		return
	}
}
