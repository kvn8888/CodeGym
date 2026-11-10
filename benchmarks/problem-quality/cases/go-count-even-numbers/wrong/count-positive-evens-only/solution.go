package main

func CountEven(numbers []int) int {
	count := 0
	for _, number := range numbers {
		if number%2 == 0 && number > 0 {
			count++
		}
	}
	return count
}
