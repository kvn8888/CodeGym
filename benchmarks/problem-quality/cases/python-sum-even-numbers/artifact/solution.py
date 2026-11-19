def sum_even(numbers: list[int]) -> int:
    total = 0
    for number in numbers:
        if number % 2 == 0:
            total += number
    return total
