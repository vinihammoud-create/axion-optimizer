package drivers

import "testing"

// Windows adds the memory size to some names; NVIDIA's list does not (the
// first notebook report: "NVIDIA GeForce RTX 3050 6GB Laptop GPU").
func TestProductIDMemorySize(t *testing.T) {
	products := map[string]int{
		"geforce rtx 3050 laptop gpu": 1001,
		"geforce gtx 1060":            1002,
		"geforce rtx 4060 laptop gpu": 1003,
		"geforce gtx 1650 ti":         1004,
		"geforce rtx 3060":            1005,
		"geforce rtx 2050":            1006,
	}
	for name, want := range map[string]int{
		"NVIDIA GeForce RTX 3050 6GB Laptop GPU":       1001,
		"NVIDIA GeForce RTX 3050 4 GB Laptop GPU":      1001,
		"NVIDIA GeForce GTX 1060 6GB":                  1002,
		"NVIDIA GeForce RTX 4060 Laptop GPU":           1003,
		"NVIDIA GeForce GTX 1650 Ti with Max-Q Design": 1004,
		"NVIDIA GeForce RTX 3060":                      1005,
		"NVIDIA GeForce RTX 2050":                      1006,
		"NVIDIA GeForce RTX 3070":                      0,
	} {
		got, ok := ProductID(products, name)
		if got != want || ok != (want != 0) {
			t.Errorf("%s: %d %v, esperado %d", name, got, ok, want)
		}
	}
}
