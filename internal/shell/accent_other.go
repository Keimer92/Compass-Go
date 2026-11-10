//go:build !windows

package shell

// countEarlierInstances no aplica fuera de Windows.
func countEarlierInstances() int { return 0 }
