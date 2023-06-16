package main

import (
	"fmt"
	"time"

	"code-intelligence.com/cifuzz/internal/names"
)

func main() {
	input := []byte(time.Now().Format("2006-01-02"))
	fmt.Println(names.GetDeterministicName(input))
}
