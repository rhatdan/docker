package mode

import (
	"testing"
)

func TestModeSpec(t *testing.T) {
	valid_ro_opts := []string{
		"/vol1:/vol1:ro",
		"/vol1:/vol1:roZ",
		"/vol1:/vol1:rZ",
	}

	valid_rw_opts := []string{
		"/vol1:/vol1",
		"/vol1:/vol1:rw",
		"/vol1:/vol1:rwZ",
		"/vol1:/vol1:zrw",
		"/vol1:/vol1:z",
		"/vol1:/vol1:Z",
		"/vol1:/vol1:wZ",
	}
	for _, opt := range valid_rw_opts {
		_, _, mode, err := parseBindMountSpec(opt)
		if err != nil {
			t.Fatal(err)
		}
		if !m.Writable() {
			t.Fatal("Writable option not writable")
		}
	}
	for _, opt := range valid_ro_opts {
		_, _, mode, err := parseBindMountSpec(opt)
		if err != nil {
			t.Fatal(err)
		}
		if volumes.Writable(mode) {
			t.Fatal("Read/Only option writable")
		}
	}

	_, _, _, err := parseBindMountSpec("/vol1:/vol1:rp")
	if err == nil {
		t.Fatal("Error should not be nil")
	}
}
