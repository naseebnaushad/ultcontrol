package main

import "fmt"

// add takes two numbers and returns their sum.
func add(a int, b int) int {
	return a + b
}

// checksum adds up all the bytes. A byte can only hold 0-255, so the total
// wraps around automatically (this is exactly what the headphones expect).
func checksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

type Battery struct {
	Level    int
	Charging bool
}

func (b Battery) Describe() string {
	if b.Charging {
		return fmt.Sprintf("%d%% (charging)", b.Level)
	}
	return fmt.Sprintf("%d%%", b.Level)
}

func main() {
	fmt.Println(add(2, 3))

	data := []byte{0x04, 0x01, 0xFF}
	fmt.Printf("% x\n", data)
	fmt.Println(checksum(data))

	b := Battery{Level: 81}
	fmt.Println(b.Describe())
}
