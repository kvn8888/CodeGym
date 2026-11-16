def sum_even(numbers: list[int]) -> int:
    return sum(number for number in numbers if number % 2 != 0)
