package main

import "testing"

func TestIsLowEntropy(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"nil", nil, true},
		{"empty", []byte{}, true},
		{"all zeros", make([]byte, 32), true},
		{"all 0xFF", bytes32(0xFF), true},
		{"all same non-zero", bytes32(0xAB), true},
		{"random-like", []byte{0x01, 0x02, 0x03, 0x04}, false},
		{"one diff byte", append(make([]byte, 31), 0x01), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isLowEntropy(tc.in)
			if got != tc.want {
				t.Errorf("isLowEntropy(%x) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func bytes32(b byte) []byte {
	buf := make([]byte, 32)
	for i := range buf {
		buf[i] = b
	}
	return buf
}
