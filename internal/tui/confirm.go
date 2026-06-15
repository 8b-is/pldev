package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm prints prompt and reads y/N from stdin. Returns true only on y/yes.
func Confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(sc.Text()))
	return a == "y" || a == "yes"
}
