package provider

import "fmt"

// humanBytes renders a byte count with a binary unit and one decimal place
// where it helps ("13.1 GB", "640 GB").
func humanBytes(bytes float64) string {
	const unit = 1024.0
	if bytes < unit {
		return fmt.Sprintf("%.0f B", bytes)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	value := bytes
	index := -1
	for value >= unit && index < len(units)-1 {
		value /= unit
		index++
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, units[index])
	}
	return fmt.Sprintf("%.1f %s", value, units[index])
}
