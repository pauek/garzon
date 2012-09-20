package main

import (
	"fmt"
)

func main() {
	var n int
	fmt.Scanf("%d", &n)
	if n == 42 {
		fmt.Printf("Accept\n")
	} else {
		fmt.Printf("Reject\n")
	}
}